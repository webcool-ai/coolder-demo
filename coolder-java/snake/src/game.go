package main

import (
	"fmt"
	"math/rand"
	"time"
)

// ---------- 网格与常量 ----------

const (
	GridW       = 36 // 棋盘宽（格）
	GridH       = 24 // 棋盘高（格）
	StartLen    = 4  // 蛇初始长度
	FoodPoints  = 10 // 每个食物得分
	MaxPlayers  = 8  // 每房间最大玩家数
	MinFood     = 1  // 食物数量下限（滑杆最小值）
	MaxFood     = 50 // 食物数量上限（滑杆最大值）
	DefaultFood = 10 // 食物数量默认值
)

// ---------- 方向 ----------

type Dir int

const (
	DirUp Dir = iota
	DirDown
	DirLeft
	DirRight
)

// dirVec: up/down/left/right 的位移。注意 up^1==down、left^1==right，
// 因此 oppositeDir 可用异或判断。
var dirVec = [4]Cell{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

var dirNames = map[string]Dir{
	"up": DirUp, "down": DirDown, "left": DirLeft, "right": DirRight,
}

type Cell struct{ X, Y int }

func (c Cell) add(d Dir) Cell {
	v := dirVec[d]
	return Cell{c.X + v.X, c.Y + v.Y}
}

func parseDir(s string) (Dir, bool) {
	d, ok := dirNames[s]
	return d, ok
}

func oppositeDir(a, b Dir) bool { return a >= DirUp && a <= DirRight && a^1 == b }

// ---------- 速度（慢 / 中 / 快） ----------

var speedIntervals = map[string]time.Duration{
	"slow":   300 * time.Millisecond, // 慢速
	"normal": 160 * time.Millisecond, // 中速
	"fast":   90 * time.Millisecond,  // 快速
}

func normalizeSpeed(s string) string {
	if _, ok := speedIntervals[s]; ok {
		return s
	}
	return "normal"
}

func speedInterval(s string) time.Duration {
	if d, ok := speedIntervals[s]; ok {
		return d
	}
	return speedIntervals["normal"]
}

func clampFoodTarget(n int) int {
	if n < MinFood {
		return MinFood
	}
	if n > MaxFood {
		return MaxFood
	}
	return n
}

// ---------- 蛇 ----------

type Snake struct {
	ID     uint32
	Name   string
	Color  string
	Cells  []Cell // Cells[0] 为蛇头
	Dir    Dir
	queue  []Dir // 方向输入队列，每帧应用一个
	Score  int
	Deaths int
	Alive  bool
}

func (s *Snake) Head() Cell { return s.Cells[0] }

// EnqueueDir 把一次按键方向放入队列（服务端循环协程内调用）。
// 规则：不能与当前方向或队尾方向相反/相同；队列最多 3 个。
func (s *Snake) EnqueueDir(d Dir) bool {
	if d < DirUp || d > DirRight {
		return false
	}
	last := s.Dir
	if len(s.queue) > 0 {
		last = s.queue[len(s.queue)-1]
	}
	if d == last || oppositeDir(d, last) {
		return false
	}
	if len(s.queue) >= 3 {
		return false
	}
	s.queue = append(s.queue, d)
	return true
}

func (s *Snake) applyQueuedDir() {
	for len(s.queue) > 0 {
		d := s.queue[0]
		s.queue = s.queue[1:]
		if d != s.Dir && !oppositeDir(d, s.Dir) {
			s.Dir = d
			return
		}
	}
}

// ---------- 世界 ----------

type World struct {
	W, H       int
	Snakes     []*Snake
	Foods      []Cell
	FoodTarget int    // 房间食物数量目标（滑杆配置）
	Speed      string // slow / normal / fast
	Running    bool   // false = 全房间暂停
	tickCount  uint64
}

func NewWorld() *World {
	return &World{
		W:          GridW,
		H:          GridH,
		FoodTarget: DefaultFood,
		Speed:      "normal",
		Running:    true,
	}
}

func (w *World) snakeByID(id uint32) *Snake {
	for _, s := range w.Snakes {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (w *World) removeSnake(id uint32) {
	for i, s := range w.Snakes {
		if s.ID == id {
			w.Snakes = append(w.Snakes[:i], w.Snakes[i+1:]...)
			return
		}
	}
}

// occupied 单元格是否被任何蛇占据（含死亡尸体）。
func (w *World) occupied(c Cell) bool {
	for _, s := range w.Snakes {
		for _, b := range s.Cells {
			if b == c {
				return true
			}
		}
	}
	return false
}

func (w *World) foodAt(c Cell) bool {
	for _, f := range w.Foods {
		if f == c {
			return true
		}
	}
	return false
}

func (w *World) removeFoodAt(c Cell) {
	for i, f := range w.Foods {
		if f == c {
			w.Foods = append(w.Foods[:i], w.Foods[i+1:]...)
			return
		}
	}
}

var colorPalette = []string{
	"#4fc3f7", "#ffd54f", "#81c784", "#f06292",
	"#ba68c8", "#ffb74d", "#4db6ac", "#e57373",
}

// pickColor 选一个当前未被使用的颜色。
func (w *World) pickColor() string {
	used := make(map[string]bool, len(w.Snakes))
	for _, s := range w.Snakes {
		used[s.Color] = true
	}
	for _, c := range colorPalette {
		if !used[c] {
			return c
		}
	}
	return colorPalette[len(w.Snakes)%len(colorPalette)]
}

// fitSpot 检查以 head 为蛇头、朝向 d 的完整蛇身能否放下。
func (w *World) fitSpot(head Cell, d Dir) ([]Cell, bool) {
	cells := make([]Cell, 0, StartLen)
	for i := 0; i < StartLen; i++ {
		c := Cell{head.X - dirVec[d].X*i, head.Y - dirVec[d].Y*i}
		if c.X < 0 || c.X >= w.W || c.Y < 0 || c.Y >= w.H || w.occupied(c) || w.foodAt(c) {
			return nil, false
		}
		cells = append(cells, c)
	}
	return cells, true
}

// freeSpot 随机找一个能放下整条蛇的位置，失败则线性扫描兜底。
func (w *World) freeSpot() ([]Cell, Dir, bool) {
	for tries := 0; tries < 400; tries++ {
		d := Dir(rand.Intn(4))
		head := Cell{rand.Intn(w.W), rand.Intn(w.H)}
		if cells, ok := w.fitSpot(head, d); ok {
			return cells, d, true
		}
	}
	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			for d := DirUp; d <= DirRight; d++ {
				if cells, ok := w.fitSpot(Cell{x, y}, d); ok {
					return cells, d, true
				}
			}
		}
	}
	return nil, DirUp, false
}

// SpawnSnake 新玩家加入。
func (w *World) SpawnSnake(id uint32, name, color string) *Snake {
	s := &Snake{ID: id, Name: name, Color: color}
	if cells, d, ok := w.freeSpot(); ok {
		s.Cells = cells
		s.Dir = d
	} else {
		s.Cells = []Cell{{1, 1}} // 理论上不可达（864 格足够）
		s.Dir = DirRight
	}
	s.Alive = true
	w.Snakes = append(w.Snakes, s)
	return s
}

// RespawnSnake 死亡后按空格重新开始：换位置重生、清空队列、清零得分。
func (w *World) RespawnSnake(s *Snake) bool {
	cells, d, ok := w.freeSpot()
	if !ok {
		return false
	}
	s.Cells = cells
	s.Dir = d
	s.queue = nil
	s.Alive = true
	s.Score = 0
	return true
}

func (w *World) spawnFood() bool {
	for tries := 0; tries < 300; tries++ {
		c := Cell{rand.Intn(w.W), rand.Intn(w.H)}
		if !w.occupied(c) && !w.foodAt(c) {
			w.Foods = append(w.Foods, c)
			return true
		}
	}
	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			c := Cell{x, y}
			if !w.occupied(c) && !w.foodAt(c) {
				w.Foods = append(w.Foods, c)
				return true
			}
		}
	}
	return false
}

// nextCells 计算移动后的蛇身：头进尾出；吃到食物则长度 +1。
func (w *World) nextCells(s *Snake, head Cell, grow bool) []Cell {
	cells := make([]Cell, 0, len(s.Cells)+1)
	cells = append(cells, head)
	cells = append(cells, s.Cells...)
	if !grow {
		cells = cells[:len(cells)-1]
	}
	return cells
}

// Tick 推进一帧（仅在 Running 时生效），返回需要广播的事件文案。
//
// 两阶段判定保证多人同帧碰撞公平：
//  1. 先计算所有存活蛇的意图位置并互相判定死亡（含迎头相撞/交换位置）
//  2. 再统一应用：存活者位移+吃食物，死亡者原地留尸
func (w *World) Tick() []string {
	if !w.Running {
		return nil
	}
	w.tickCount++
	events := make([]string, 0, 4)

	type pend struct {
		s     *Snake
		head  Cell
		grow  bool
		cells []Cell
	}

	alive := make([]*Snake, 0, len(w.Snakes))
	for _, s := range w.Snakes {
		if s.Alive {
			alive = append(alive, s)
		}
	}

	pends := make([]pend, 0, len(alive))
	for _, s := range alive {
		s.applyQueuedDir()
		head := s.Head().add(s.Dir)
		grow := w.foodAt(head)
		pends = append(pends, pend{s: s, head: head, grow: grow, cells: w.nextCells(s, head, grow)})
	}

	// 第一阶段：死亡判定（基于所有人的新身体，新身体第 1 段即旧蛇头，
	// 因此"移动到对方让出的旧头位置"这类交换相撞也能被正确判定）。
	reasons := make([]string, len(pends))
	for i := range pends {
		p := &pends[i]
		h := p.head
		if h.X < 0 || h.X >= w.W || h.Y < 0 || h.Y >= w.H {
			reasons[i] = "撞墙了"
			continue
		}
		for j := range pends {
			q := &pends[j]
			start := 0
			if q.s == p.s {
				start = 1 // 跳过自己的新蛇头
			}
			for k := start; k < len(q.cells); k++ {
				if q.cells[k] == h {
					reasons[i] = "撞到了蛇身"
					break
				}
			}
			if reasons[i] != "" {
				break
			}
		}
	}

	// 第二阶段：应用结果。
	for i := range pends {
		p := &pends[i]
		if reasons[i] != "" {
			p.s.Alive = false
			p.s.Deaths++
			p.s.queue = nil
			events = append(events, fmt.Sprintf("💀 %s %s", p.s.Name, reasons[i]))
			continue
		}
		p.s.Cells = p.cells
		if p.grow {
			w.removeFoodAt(p.head)
			p.s.Score += FoodPoints
		}
	}

	// 食物数量向滑杆目标渐进靠拢。
	w.adjustFood()
	return events
}

// adjustFood 每帧把食物数量向 FoodTarget 靠拢（最多 ±2~3 个，视觉平滑）。
func (w *World) adjustFood() {
	deficit := w.FoodTarget - len(w.Foods)
	spawn := 0
	switch {
	case deficit >= 10:
		spawn = 3
	case deficit >= 3:
		spawn = 2
	case deficit >= 1:
		spawn = 1
	}
	for i := 0; i < spawn; i++ {
		w.spawnFood()
	}
	if deficit < 0 {
		remove := -deficit
		if remove > 2 {
			remove = 2
		}
		for i := 0; i < remove && len(w.Foods) > 0; i++ {
			j := rand.Intn(len(w.Foods))
			w.Foods = append(w.Foods[:j], w.Foods[j+1:]...)
		}
	}
}
