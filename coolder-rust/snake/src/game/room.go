package game

import (
	"encoding/json"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"example.com/snake/src/ws"
)

const (
	// TickInterval is how often every room simulates and broadcasts.
	TickInterval = 50 * time.Millisecond

	// GridW / GridH define the playfield in cells.
	GridW = 42
	GridH = 28

	// DefaultFood is the food count of a fresh room.
	DefaultFood = 5

	// MaxFoodPerRoom caps the food slider.
	MaxFoodPerRoom = 40

	// InitialLength is the length of a freshly spawned snake.
	InitialLength = 3

	// CorpseTicks keeps a dead snake visible for ~1.2s before it fades.
	CorpseTicks = 24

	// roomIdleTTL is how long an empty room survives before the
	// janitor reclaims it.
	roomIdleTTL = 10 * time.Minute
)

// Client couples a websocket connection to the snake it controls.
type Client struct {
	conn  *ws.Conn
	send  chan []byte
	snake *Snake
}

// writer pumps outgoing messages; a write error tears the connection down.
func (c *Client) writer() {
	for msg := range c.send {
		if err := c.conn.WriteText(msg); err != nil {
			c.conn.Close()
			return
		}
	}
}

// trySend queues a message without ever blocking the game loop.
func (c *Client) trySend(msg []byte) {
	select {
	case c.send <- msg:
	default: // client cannot keep up; dropping one frame is fine
	}
}

// RoomInfo is the lobby view of a room.
type RoomInfo struct {
	Name    string `json:"name"`
	Players int    `json:"players"`
}

// Room is one independent match; every room runs its own loop.
type Room struct {
	name string
	w, h int

	mu         sync.Mutex
	food       []Point
	foodCnt    int
	snakes     map[string]*Snake
	order      []string // snake IDs in join order (stable rendering)
	clients    map[*Client]struct{}
	nextID     int
	tick       int
	running    bool
	reserved   int       // in-use marks taken by AcquireRoom (janitor guard)
	emptySince time.Time // when the room last became empty
}

// Manager owns all rooms and glues websocket connections to them.
type Manager struct {
	mu      sync.RWMutex
	rooms   map[string]*Room
	w, h    int
	defFood int
}

// NewManager creates the room registry and starts the idle-room janitor.
func NewManager(w, h, defFood int) *Manager {
	m := &Manager{rooms: make(map[string]*Room), w: w, h: h, defFood: defFood}
	go m.janitor()
	return m
}

// AcquireRoom returns the named room (creating it on first use) and
// marks it in use so the janitor cannot reclaim it mid-join.
func (m *Manager) AcquireRoom(name string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rooms[name]
	if !ok {
		r = &Room{
			name:    name,
			w:       m.w,
			h:       m.h,
			foodCnt: m.defFood,
			snakes:  make(map[string]*Snake),
			clients: make(map[*Client]struct{}),
		}
		m.rooms[name] = r
	}
	r.reserved++
	return r
}

// ReleaseRoom drops one in-use mark taken by AcquireRoom.
func (m *Manager) ReleaseRoom(r *Room) {
	m.mu.Lock()
	r.reserved--
	m.mu.Unlock()
}

// janitor periodically reclaims rooms that have been empty for a while.
func (m *Manager) janitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		m.gcRooms()
	}
}

func (m *Manager) gcRooms() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for name, r := range m.rooms {
		r.mu.Lock()
		idle := r.reserved == 0 &&
			len(r.clients) == 0 &&
			!r.running &&
			!r.emptySince.IsZero() &&
			now.Sub(r.emptySince) > roomIdleTTL
		r.mu.Unlock()
		if idle {
			delete(m.rooms, name)
		}
	}
}

// List snapshots all rooms for the lobby endpoint.
func (m *Manager) List() []RoomInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]RoomInfo, 0, len(m.rooms))
	for _, r := range m.rooms {
		r.mu.Lock()
		n := len(r.snakes)
		r.mu.Unlock()
		out = append(out, RoomInfo{Name: r.name, Players: n})
	}
	return out
}

// Serve joins a freshly upgraded websocket to a room and pumps client
// messages until the connection drops.
func (m *Manager) Serve(conn *ws.Conn, roomName, playerName, speed string) {
	room := m.AcquireRoom(roomName)
	defer m.ReleaseRoom(room)

	cl := room.addClient(conn, playerName, speed)
	defer room.removeClient(cl)

	for {
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg ClientMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		room.handleClient(cl, &msg)
	}
}

// ---------------------------------------------------------------- room

func (r *Room) addClient(conn *ws.Conn, name, speed string) *Client {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	if name == "" {
		name = "玩家" + strconv.Itoa(r.nextID)
	}
	s := &Snake{
		ID:    "p" + strconv.Itoa(r.nextID),
		Name:  name,
		Color: colorFor(r.nextID),
		Speed: NormalizeSpeed(speed),
		Alive: true,
	}
	r.spawnSnake(s)
	r.snakes[s.ID] = s
	r.order = append(r.order, s.ID)

	cl := &Client{conn: conn, send: make(chan []byte, 128), snake: s}
	r.clients[cl] = struct{}{}
	r.emptySince = time.Time{}

	// Queue the welcome frame before the first state frame so the
	// client always learns its identity first. Going through the send
	// channel (instead of writing on the socket directly) keeps the
	// room lock free of potentially slow socket writes.
	cl.trySend(mustJSON(WelcomeMsg{
		Type:      MsgWelcome,
		ID:        s.ID,
		Room:      r.name,
		Width:     r.w,
		Height:    r.h,
		FoodCount: r.foodCnt,
		MaxFood:   MaxFoodPerRoom,
		Speed:     string(s.Speed),
	}))
	go cl.writer()

	r.broadcastLocked()

	if !r.running {
		r.running = true
		go r.loop()
	}
	return cl
}

func (r *Room) removeClient(cl *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.clients[cl]; !ok {
		return
	}
	delete(r.clients, cl)
	close(cl.send)
	if s := cl.snake; s != nil {
		delete(r.snakes, s.ID)
		for i, id := range r.order {
			if id == s.ID {
				r.order = append(r.order[:i], r.order[i+1:]...)
				break
			}
		}
	}
	if len(r.clients) == 0 {
		r.emptySince = time.Now()
	}
}

func (r *Room) loop() {
	ticker := time.NewTicker(TickInterval)
	defer ticker.Stop()
	for range ticker.C {
		if !r.step() {
			return
		}
	}
}

// step simulates one tick. It returns false once the room is empty so
// the loop goroutine can stop.
func (r *Room) step() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.clients) == 0 {
		r.running = false
		r.emptySince = time.Now()
		return false
	}
	r.tick++
	now := time.Now()

	r.maintainFood()

	for _, id := range r.order {
		s := r.snakes[id]
		if s == nil || !s.Alive {
			continue
		}
		if !s.Paused && !now.Before(s.next) {
			r.stepSnake(s, now)
		}
	}

	// dead corpses fade out after CorpseTicks
	for _, s := range r.snakes {
		if !s.Alive && len(s.Body) > 0 {
			s.deadTicks++
			if s.deadTicks >= CorpseTicks {
				s.Body = nil
			}
		}
	}

	r.broadcastLocked()
	return true
}

// maintainFood adjusts the food list to the configured count.
func (r *Room) maintainFood() {
	if len(r.food) < r.foodCnt {
		occ := r.occupiedSet(true)
		for len(r.food) < r.foodCnt {
			p, ok := r.randomFreeCell(occ)
			if !ok {
				break
			}
			occ[p] = true
			r.food = append(r.food, p)
		}
	}
	for len(r.food) > r.foodCnt {
		r.food = r.food[:len(r.food)-1]
	}
}

func (r *Room) occupiedSet(includeFood bool) map[Point]bool {
	occ := make(map[Point]bool, 128)
	for _, s := range r.snakes {
		for _, p := range s.Body {
			occ[p] = true
		}
	}
	if includeFood {
		for _, f := range r.food {
			occ[f] = true
		}
	}
	return occ
}

func (r *Room) randomFreeCell(occ map[Point]bool) (Point, bool) {
	for i := 0; i < 400; i++ {
		p := Point{X: rand.Intn(r.w), Y: rand.Intn(r.h)}
		if !occ[p] {
			return p, true
		}
	}
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			p := Point{X: x, Y: y}
			if !occ[p] {
				return p, true
			}
		}
	}
	return Point{}, false
}

// spawnSnake places a new or restarting snake on a free straight run.
func (r *Room) spawnSnake(s *Snake) {
	occ := r.occupiedSet(true)
	dirs := []Point{{X: 1, Y: 0}, {X: -1, Y: 0}, {X: 0, Y: 1}, {X: 0, Y: -1}}
	for i := 0; i < 300; i++ {
		p := Point{X: rand.Intn(r.w), Y: rand.Intn(r.h)}
		if occ[p] {
			continue
		}
		rand.Shuffle(len(dirs), func(a, b int) { dirs[a], dirs[b] = dirs[b], dirs[a] })
		for _, d := range dirs {
			clearRun := true
			for k := 1; k < InitialLength; k++ {
				q := Point{X: p.X - d.X*k, Y: p.Y - d.Y*k}
				if q.X < 0 || q.Y < 0 || q.X >= r.w || q.Y >= r.h || occ[q] {
					clearRun = false
					break
				}
			}
			if !clearRun {
				continue
			}
			s.Dir = d
			s.Body = []Point{p}
			for k := 1; k < InitialLength; k++ {
				s.Body = append(s.Body, Point{X: p.X - d.X*k, Y: p.Y - d.Y*k})
			}
			s.Pending = nil
			s.Grow = 0
			s.next = time.Now().Add(SpeedInterval(s.Speed))
			return
		}
	}
	// Grid is nearly full: fall back to any free cell.
	if p, ok := r.randomFreeCell(occ); ok {
		s.Dir = Point{X: 1, Y: 0}
		s.Body = []Point{p}
		s.next = time.Now().Add(SpeedInterval(s.Speed))
	}
}

// stepSnake advances one snake a single cell.
func (r *Room) stepSnake(s *Snake, now time.Time) {
	if !s.Alive || s.Paused || len(s.Body) == 0 {
		return
	}

	// consume queued direction changes, skipping illegal 180-degree turns
	for len(s.Pending) > 0 {
		d := s.Pending[0]
		s.Pending = s.Pending[1:]
		if d == s.Dir || (d.X == -s.Dir.X && d.Y == -s.Dir.Y) {
			continue
		}
		s.Dir = d
		break
	}

	head := Point{X: s.Body[0].X + s.Dir.X, Y: s.Body[0].Y + s.Dir.Y}

	// walls kill
	if head.X < 0 || head.Y < 0 || head.X >= r.w || head.Y >= r.h {
		r.kill(s)
		return
	}

	// any snake body (own vacating tail excluded) kills
	for _, other := range r.snakes {
		for i, cell := range other.Body {
			if cell != head {
				continue
			}
			if i == len(other.Body)-1 && other.Grow == 0 {
				continue // tail is about to vacate this cell
			}
			r.kill(s)
			return
		}
	}

	// food
	ate := -1
	for i, f := range r.food {
		if f == head {
			ate = i
			break
		}
	}

	s.Body = append([]Point{head}, s.Body...)
	if ate >= 0 {
		r.food = append(r.food[:ate], r.food[ate+1:]...)
		s.Score++
		s.Grow += 2
	}
	if s.Grow > 0 {
		s.Grow--
	} else {
		s.Body = s.Body[:len(s.Body)-1]
	}
	s.next = now.Add(SpeedInterval(s.Speed))
}

func (r *Room) kill(s *Snake) {
	s.Alive = false
	s.Paused = false
	s.Pending = nil
	s.deadTicks = 0
}

// handleClient applies one client message to the room state.
func (r *Room) handleClient(cl *Client, msg *ClientMsg) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := cl.snake
	if s == nil {
		return
	}
	switch msg.Type {
	case MsgDir:
		if !s.Alive || s.Paused {
			return
		}
		d := Point{X: msg.DX, Y: msg.DY}
		if (d.X == 0 && d.Y == 0) || len(s.Pending) >= 2 {
			return
		}
		last := s.Dir
		if len(s.Pending) > 0 {
			last = s.Pending[len(s.Pending)-1]
		}
		if d == last || (d.X == -last.X && d.Y == -last.Y) {
			return
		}
		s.Pending = append(s.Pending, d)

	case MsgSpeed:
		sp := NormalizeSpeed(msg.Speed)
		if sp != s.Speed {
			s.Speed = sp
			s.next = time.Now().Add(SpeedInterval(sp))
		}

	case MsgFood:
		n := msg.Food
		if n < 0 {
			n = 0
		}
		if n > MaxFoodPerRoom {
			n = MaxFoodPerRoom
		}
		r.foodCnt = n // takes effect on the next tick

	case MsgPause:
		if !s.Alive {
			return
		}
		s.Paused = msg.Paused
		if !s.Paused {
			s.next = time.Now().Add(SpeedInterval(s.Speed))
		}

	case MsgRestart:
		if !s.Alive {
			s.Alive = true
			s.Score = 0
			s.Grow = 0
			s.deadTicks = 0
			s.Paused = false
			r.spawnSnake(s)
		}

	case MsgPing:
		// keepalive only; the read deadline is refreshed by the read loop
	}
}

// broadcastLocked sends a full state snapshot to every connected client.
// The caller must hold r.mu.
func (r *Room) broadcastLocked() {
	views := make([]*SnakeView, 0, len(r.order))
	for _, id := range r.order {
		s := r.snakes[id]
		if s == nil {
			continue
		}
		body := make([]Point, len(s.Body))
		copy(body, s.Body)
		views = append(views, &SnakeView{
			ID:     s.ID,
			Name:   s.Name,
			Body:   body,
			Dir:    s.Dir,
			Color:  s.Color,
			Score:  s.Score,
			Alive:  s.Alive,
			Paused: s.Paused,
			Speed:  string(s.Speed),
		})
	}
	food := make([]Point, len(r.food))
	copy(food, r.food)
	payload := mustJSON(StateMsg{Type: MsgState, Tick: r.tick, Food: food, Snakes: views})
	for cl := range r.clients {
		cl.trySend(payload)
	}
}
