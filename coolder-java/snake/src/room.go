package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

var playerSeq uint32

const sendBuf = 64

// ---------- 玩家 ----------

type Player struct {
	ID   uint32
	Name string

	conn *WSConn
	room *Room
	send chan []byte
	done chan struct{}
	once sync.Once
}

// sendMsg 向玩家投递消息：连接已关则丢弃，缓冲满也丢弃（下一帧状态会覆盖）。
func (p *Player) sendMsg(b []byte) {
	select {
	case <-p.done:
		return
	default:
	}
	select {
	case p.send <- b:
	default:
	}
}

func (p *Player) close() {
	p.once.Do(func() { close(p.done) })
	p.conn.Close()
}

// writeLoop 每玩家一个写协程：从 send 通道取消息写帧，并定期发 Ping 保活。
func (p *Player) writeLoop() {
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case msg := <-p.send:
			if err := p.conn.WriteText(msg); err != nil {
				p.close()
				return
			}
		case <-ping.C:
			if err := p.conn.WritePing(); err != nil {
				p.close()
				return
			}
		case <-p.done:
			return
		}
	}
}

// readLoop 每玩家一个读协程：解析客户端消息并投递到房间邮箱。
func (p *Player) readLoop() {
	defer p.leaveRoom()
	for {
		data, err := p.conn.ReadMessage()
		if err != nil {
			return
		}
		var m struct {
			Type  string `json:"type"`
			Dir   string `json:"dir"`
			Speed string `json:"speed"`
			Count int    `json:"count"`
		}
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.Type {
		case "dir":
			if d, ok := parseDir(m.Dir); ok {
				p.room.post(roomCmd{kind: cmdDir, player: p, dir: d})
			}
		case "space": // 空格：暂停/继续；死亡时=重新开始
			p.room.post(roomCmd{kind: cmdSpace, player: p})
		case "speed": // 慢/中/快
			p.room.post(roomCmd{kind: cmdSpeed, player: p, speed: normalizeSpeed(m.Speed)})
		case "food": // 滑杆：食物数量
			p.room.post(roomCmd{kind: cmdFood, player: p, count: m.Count})
		case "ping":
			p.sendMsg([]byte(`{"type":"pong"}`))
		}
	}
}

func (p *Player) leaveRoom() {
	p.close()
	p.room.post(roomCmd{kind: cmdLeave, player: p})
}

// ---------- 房间命令 ----------

const (
	cmdJoin = iota
	cmdLeave
	cmdDir
	cmdSpace
	cmdSpeed
	cmdFood
)

type roomCmd struct {
	kind   int
	player *Player
	dir    Dir
	speed  string
	count  int
}

// ---------- 房间 ----------

type Room struct {
	Name    string
	mailbox chan roomCmd
	world   *World
	mgr     *Manager

	mu          sync.Mutex
	players     map[*Player]bool
	closed      bool
	infoRunning bool // 供 /api/rooms 无锁读取（仅循环协程修改）
	infoSpeed   string

	ticker *time.Ticker
}

func newRoom(name string, mgr *Manager) *Room {
	w := NewWorld()
	return &Room{
		Name:        name,
		mailbox:     make(chan roomCmd, 128),
		world:       w,
		mgr:         mgr,
		players:     make(map[*Player]bool),
		infoRunning: w.Running,
		infoSpeed:   w.Speed,
	}
}

// run 房间主循环：处理命令邮箱 → 每帧推进世界 → 广播快照。
func (r *Room) run() {
	r.ticker = time.NewTicker(speedInterval(r.world.Speed))
	defer r.ticker.Stop()
	for {
		select {
		case c := <-r.mailbox:
			r.handleCmd(c)
			if r.idle() {
				r.shutdown()
				return
			}
		case <-r.ticker.C:
		drain: // 帧到时把积压命令全部处理掉再推进
			for {
				select {
				case c := <-r.mailbox:
					r.handleCmd(c)
				default:
					break drain
				}
			}
			r.tickOnce()
			if r.idle() {
				r.shutdown()
				return
			}
		}
	}
}

// idle：没有任何玩家且邮箱里也没有待处理的 join。
func (r *Room) idle() bool {
	if r.playerCount() > 0 {
		return false
	}
	return len(r.mailbox) == 0
}

func (r *Room) post(c roomCmd) {
	select {
	case r.mailbox <- c:
	default: // 邮箱满时兜底异步投递，避免读协程永久阻塞
		go func() {
			select {
			case r.mailbox <- c:
			case <-time.After(5 * time.Second):
			}
		}()
	}
}

func (r *Room) playerCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.players)
}

func (r *Room) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

func (r *Room) info() (players int, running bool, speed string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.players), r.infoRunning, r.infoSpeed
}

func (r *Room) setRunning(v bool) {
	r.mu.Lock()
	r.infoRunning = v
	r.mu.Unlock()
	r.world.Running = v
}

func (r *Room) setSpeed(s string) {
	r.mu.Lock()
	r.infoSpeed = s
	r.mu.Unlock()
	r.world.Speed = s
}

func (r *Room) handleCmd(c roomCmd) {
	switch c.kind {
	case cmdJoin:
		r.mu.Lock()
		_, inRoom := r.players[c.player]
		r.mu.Unlock()
		if !inRoom {
			return // 玩家已断开（leave 先于 join 被处理）
		}
		if len(r.world.Snakes) >= MaxPlayers {
			c.player.sendMsg([]byte(`{"type":"error","text":"房间已满（最多 8 人）"}`))
			c.player.close()
			r.mu.Lock()
			delete(r.players, c.player)
			r.mu.Unlock()
			return
		}
		r.world.SpawnSnake(c.player.ID, c.player.Name, r.world.pickColor())
		c.player.sendMsg(marshalState(r.world, "init", c.player.ID))
		r.broadcastEvent(fmt.Sprintf("👋 %s 加入了房间", c.player.Name))
		r.broadcastState()

	case cmdLeave:
		r.mu.Lock()
		_, inRoom := r.players[c.player]
		delete(r.players, c.player)
		r.mu.Unlock()
		if !inRoom {
			return
		}
		r.world.removeSnake(c.player.ID)
		r.broadcastEvent(fmt.Sprintf("🚪 %s 离开了房间", c.player.Name))
		r.broadcastState()

	case cmdDir:
		if s := r.world.snakeByID(c.player.ID); s != nil && s.Alive {
			s.EnqueueDir(c.dir)
		}

	case cmdSpace:
		s := r.world.snakeByID(c.player.ID)
		if s == nil {
			return
		}
		if s.Alive {
			// 空格：按一次暂停，再按继续（全房间生效）
			r.setRunning(!r.world.Running)
			if r.world.Running {
				r.broadcastEvent(fmt.Sprintf("▶️ %s 继续了游戏", c.player.Name))
			} else {
				r.broadcastEvent(fmt.Sprintf("⏸️ %s 暂停了游戏", c.player.Name))
			}
			r.broadcastState()
		} else if r.world.RespawnSnake(s) {
			// 游戏失败后按空格重新开始（顺带恢复运行）
			if !r.world.Running {
				r.setRunning(true)
			}
			r.broadcastEvent(fmt.Sprintf("🔄 %s 重新开始了游戏", c.player.Name))
			r.broadcastState()
		}

	case cmdSpeed:
		if r.world.Speed != c.speed {
			r.setSpeed(c.speed)
			r.ticker.Reset(speedInterval(c.speed))
			r.broadcastEvent(fmt.Sprintf("⚡ %s 将速度调整为「%s」", c.player.Name, speedLabel(c.speed)))
			r.broadcastState()
		}

	case cmdFood:
		n := clampFoodTarget(c.count)
		if r.world.FoodTarget != n {
			r.world.FoodTarget = n
			r.broadcastEvent(fmt.Sprintf("🍎 %s 把食物数量调整为 %d", c.player.Name, n))
			r.broadcastState()
		}
	}
}

func (r *Room) tickOnce() {
	for _, e := range r.world.Tick() {
		r.broadcastEvent(e)
	}
	r.broadcastState()
}

func (r *Room) broadcastState() {
	r.broadcast(marshalState(r.world, "state", 0))
}

func (r *Room) broadcastEvent(text string) {
	b, _ := json.Marshal(map[string]string{"type": "event", "text": text})
	r.broadcast(b)
}

func (r *Room) broadcast(msg []byte) {
	r.mu.Lock()
	targets := make([]*Player, 0, len(r.players))
	for p := range r.players {
		targets = append(targets, p)
	}
	r.mu.Unlock()
	for _, p := range targets {
		p.sendMsg(msg)
	}
}

// shutdown 房间清空：踢掉迟到的连接者并从管理器注销。
func (r *Room) shutdown() {
	r.mu.Lock()
	r.closed = true
	kick := make([]*Player, 0, len(r.players))
	for p := range r.players {
		kick = append(kick, p)
		delete(r.players, p)
	}
	r.mu.Unlock()
	for _, p := range kick {
		p.sendMsg([]byte(`{"type":"error","text":"房间已关闭，请重新加入"}`))
		p.close()
	}
	r.mgr.remove(r.Name)
}

// ---------- 房间管理器 ----------

type Manager struct {
	mu    sync.Mutex
	rooms map[string]*Room
}

func NewManager() *Manager {
	return &Manager{rooms: make(map[string]*Room)}
}

// getOrCreate 找到或创建房间，并把玩家原子地登记进去（新房间随后才启动循环，
// 避免循环在玩家登记前误判空房而关闭）。
func (m *Manager) getOrCreate(name string, p *Player) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rooms[name]
	created := false
	if ok && r.isClosed() {
		delete(m.rooms, name)
		ok = false
	}
	if !ok {
		r = newRoom(name, m)
		m.rooms[name] = r
		created = true
	}
	r.mu.Lock()
	r.players[p] = true
	r.mu.Unlock()
	if created {
		go r.run()
	}
	return r
}

func (m *Manager) remove(name string) {
	m.mu.Lock()
	delete(m.rooms, name)
	m.mu.Unlock()
	log.Printf("房间 [%s] 已关闭（无人在线）", name)
}

// ---------- HTTP 处理 ----------

type roomInfo struct {
	Name    string `json:"name"`
	Players int    `json:"players"`
	Max     int    `json:"max"`
	Running bool   `json:"running"`
	Speed   string `json:"speed"`
}

// handleRooms GET /api/rooms：活跃房间列表（供大厅展示）。
func (m *Manager) handleRooms(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	list := make([]roomInfo, 0, len(m.rooms))
	for _, room := range m.rooms {
		n, running, speed := room.info()
		list = append(list, roomInfo{Name: room.Name, Players: n, Max: MaxPlayers, Running: running, Speed: speed})
	}
	m.mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(list)
}

// serveWS GET /ws?room=xxx&name=yyy：升级为 WebSocket 并加入房间。
func (m *Manager) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := Upgrade(w, r)
	if err != nil {
		http.Error(w, "需要 WebSocket 握手", http.StatusBadRequest)
		return
	}
	id := atomic.AddUint32(&playerSeq, 1)
	p := &Player{
		ID:   id,
		Name: sanitizeName(r.URL.Query().Get("name"), id),
		conn: conn,
		send: make(chan []byte, sendBuf),
		done: make(chan struct{}),
	}
	room := m.getOrCreate(sanitizeRoom(r.URL.Query().Get("room")), p)
	p.room = room

	room.post(roomCmd{kind: cmdJoin, player: p})
	go p.writeLoop()
	p.readLoop() // 阻塞直到连接断开；退出时自动离房
}

// ---------- 工具 ----------

func sanitizeName(s string, id uint32) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if utf8.RuneCountInString(s) > 12 {
		s = string([]rune(s)[:12])
	}
	if s == "" {
		s = fmt.Sprintf("玩家%d", id%1000)
	}
	return s
}

func sanitizeRoom(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > 20 {
		s = string([]rune(s)[:20])
	}
	if s == "" {
		s = "大厅"
	}
	return s
}

func speedLabel(s string) string {
	switch s {
	case "slow":
		return "慢速"
	case "fast":
		return "快速"
	default:
		return "中速"
	}
}

// ---------- 状态快照（每帧广播） ----------

type snakeJSON struct {
	ID    uint32   `json:"id"`
	Name  string   `json:"name"`
	Color string   `json:"color"`
	Score int      `json:"score"`
	Alive bool     `json:"alive"`
	Cells [][2]int `json:"cells"`
}

type stateMsg struct {
	Type       string      `json:"type"` // state | init
	Tick       uint64      `json:"tick"`
	Running    bool        `json:"running"`
	Speed      string      `json:"speed"`
	FoodTarget int         `json:"foodTarget"`
	W          int         `json:"w"`
	H          int         `json:"h"`
	You        uint32      `json:"you,omitempty"`
	Snakes     []snakeJSON `json:"snakes"`
	Foods      [][2]int    `json:"foods"`
}

func marshalState(w *World, typ string, you uint32) []byte {
	msg := stateMsg{
		Type:       typ,
		Tick:       w.tickCount,
		Running:    w.Running,
		Speed:      w.Speed,
		FoodTarget: w.FoodTarget,
		W:          w.W,
		H:          w.H,
		You:        you,
		Snakes:     make([]snakeJSON, 0, len(w.Snakes)),
		Foods:      make([][2]int, 0, len(w.Foods)),
	}
	for _, s := range w.Snakes {
		cells := make([][2]int, 0, len(s.Cells))
		for _, c := range s.Cells {
			cells = append(cells, [2]int{c.X, c.Y})
		}
		msg.Snakes = append(msg.Snakes, snakeJSON{
			ID: s.ID, Name: s.Name, Color: s.Color,
			Score: s.Score, Alive: s.Alive, Cells: cells,
		})
	}
	for _, f := range w.Foods {
		msg.Foods = append(msg.Foods, [2]int{f.X, f.Y})
	}
	b, _ := json.Marshal(msg)
	return b
}
