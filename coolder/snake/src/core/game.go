package core

import (
	"math/rand"
	"sort"
	"time"
)

// Snake 是一条蛇的全部状态。
type Snake struct {
	ID        string
	Name      string
	Body      []Point // 头部在前
	Direction Direction
	Alive     bool
	Paused    bool
	Score     int
	Speed     Speed

	pending Direction
	acc     time.Duration
}

// Game 是一个房间的独立棋盘。
type Game struct {
	width     int
	height    int
	foodCount int
	food      []Point
	snakes    map[string]*Snake
	rng       *rand.Rand
}

// NewGame 创建棋盘并按默认配置铺开食物。
func NewGame(width, height int, seed int64) *Game {
	if width < 10 {
		width = DefaultWidth
	}
	if height < 10 {
		height = DefaultHeight
	}
	game := &Game{
		width:     width,
		height:    height,
		foodCount: DefaultFoodCount,
		snakes:    make(map[string]*Snake),
		rng:       rand.New(rand.NewSource(seed)),
	}
	game.refillFood()
	return game
}

// Width 返回棋盘宽度（格）。
func (g *Game) Width() int { return g.width }

// Height 返回棋盘高度（格）。
func (g *Game) Height() int { return g.height }

// FoodCount 返回当前配置的食物数量。
func (g *Game) FoodCount() int { return g.foodCount }

// Food 返回当前所有食物位置。
func (g *Game) Food() []Point {
	out := make([]Point, len(g.food))
	copy(out, g.food)
	return out
}

// InBounds 判断坐标是否在棋盘内。
func (g *Game) InBounds(p Point) bool {
	return p.X >= 0 && p.Y >= 0 && p.X < g.width && p.Y < g.height
}

// Snake 按标识查找蛇。
func (g *Game) Snake(id string) (*Snake, bool) {
	snake, ok := g.snakes[id]
	return snake, ok
}

// SnakeIDs 返回按标识排序的蛇的 ID 列表。
func (g *Game) SnakeIDs() []string {
	ids := make([]string, 0, len(g.snakes))
	for id := range g.snakes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// AddSnake 在空闲位置生成一条新蛇，无法放置时返回 nil。
func (g *Game) AddSnake(id, name string) *Snake {
	if existing, ok := g.snakes[id]; ok {
		return existing
	}
	head, dir, ok := g.freeSpawn()
	if !ok {
		return nil
	}
	snake := newSnake(id, name, head, dir)
	g.snakes[id] = snake
	return snake
}

// RemoveSnake 移除一条蛇。
func (g *Game) RemoveSnake(id string) {
	delete(g.snakes, id)
}

// SetFoodCount 修改食物数量，返回生效后的值（范围 1..20）。
func (g *Game) SetFoodCount(count int) int {
	if count < MinFoodCount {
		count = MinFoodCount
	}
	if count > MaxFoodCount {
		count = MaxFoodCount
	}
	g.foodCount = count
	if len(g.food) > count {
		g.food = g.food[:count]
	}
	g.refillFood()
	return g.foodCount
}

// SetSpeed 设置某条蛇的速度档位。
func (g *Game) SetSpeed(id, value string) (Speed, error) {
	snake, ok := g.snakes[id]
	if !ok {
		return "", ErrNoSuchPlayer
	}
	speed, ok := ParseSpeed(value)
	if !ok {
		return "", ErrBadSpeed
	}
	snake.Speed = speed
	return speed, nil
}

// Input 处理一次按键：方向键改变朝向，空格暂停/继续或死亡后重新开始。
func (g *Game) Input(id, key string) error {
	snake, ok := g.snakes[id]
	if !ok {
		return ErrNoSuchPlayer
	}
	if key == "space" {
		g.togglePauseOrRestart(snake)
		return nil
	}
	dir, ok := DirectionForKey(key)
	if !ok {
		return ErrBadKey
	}
	if !snake.Alive || snake.Paused {
		return nil
	}
	if snake.Direction.Opposite(dir) {
		return nil
	}
	snake.pending = dir
	return nil
}

// Step 推进一个领域 tick，到期的活蛇向前移动一格。
func (g *Game) Step() {
	for _, id := range g.SnakeIDs() {
		snake := g.snakes[id]
		if !snake.Alive || snake.Paused {
			continue
		}
		interval := snake.Speed.Interval()
		snake.acc += TickDuration
		if snake.acc < interval {
			continue
		}
		snake.acc -= interval
		g.advance(snake)
	}
}

// Snapshot 生成可直接下发给前端的状态快照。
func (g *Game) Snapshot(room string) View {
	view := View{
		Room:      room,
		Width:     g.width,
		Height:    g.height,
		FoodCount: g.foodCount,
		Food:      g.Food(),
		Snakes:    make([]SnakeView, 0, len(g.snakes)),
	}
	for _, id := range g.SnakeIDs() {
		snake := g.snakes[id]
		body := make([]Point, len(snake.Body))
		copy(body, snake.Body)
		view.Snakes = append(view.Snakes, SnakeView{
			ID:     snake.ID,
			Name:   snake.Name,
			Body:   body,
			Alive:  snake.Alive,
			Paused: snake.Paused,
			Score:  snake.Score,
			Speed:  string(snake.Speed),
		})
	}
	return view
}

// newSnake 按头部位置和朝向构造一条初始长度的蛇。
func newSnake(id, name string, head Point, dir Direction) *Snake {
	dx, dy := dir.Delta()
	body := make([]Point, initialLength)
	for i := range body {
		body[i] = Point{X: head.X - dx*i, Y: head.Y - dy*i}
	}
	return &Snake{
		ID:        id,
		Name:      name,
		Body:      body,
		Direction: dir,
		pending:   dir,
		Alive:     true,
		Speed:     SpeedMedium,
	}
}

// togglePauseOrRestart 实现空格键语义：活着时切换暂停，死亡后重新开始。
func (g *Game) togglePauseOrRestart(snake *Snake) {
	if snake.Alive {
		snake.Paused = !snake.Paused
		return
	}
	head, dir, ok := g.freeSpawn()
	if !ok {
		return
	}
	snake.Body = newSnake(snake.ID, snake.Name, head, dir).Body
	snake.Direction = dir
	snake.pending = dir
	snake.Alive = true
	snake.Paused = false
	snake.Score = 0
	snake.acc = 0
}

// advance 让一条蛇前进一格，处理吃食物、撞墙与撞身体。
func (g *Game) advance(snake *Snake) {
	snake.Direction = snake.pending
	dx, dy := snake.Direction.Delta()
	head := Point{X: snake.Body[0].X + dx, Y: snake.Body[0].Y + dy}
	if !g.InBounds(head) || g.hitsBody(snake, head) {
		snake.Alive = false
		return
	}
	grew := g.takeFood(head)
	if grew {
		snake.Score += 10
	} else {
		snake.Body = snake.Body[:len(snake.Body)-1]
	}
	snake.Body = append([]Point{head}, snake.Body...)
	if grew {
		g.refillFood()
	}
}

// hitsBody 判断目标格是否撞到自身躯干或其它存活蛇。
func (g *Game) hitsBody(snake *Snake, target Point) bool {
	for i := 0; i < len(snake.Body)-1; i++ {
		if snake.Body[i] == target {
			return true
		}
	}
	for id, other := range g.snakes {
		if id == snake.ID || !other.Alive {
			continue
		}
		for _, cell := range other.Body {
			if cell == target {
				return true
			}
		}
	}
	return false
}

// occupied 判断格子是否已被存活蛇的身体占用。
func (g *Game) occupied(target Point, ignoreID string) bool {
	for id, snake := range g.snakes {
		if id == ignoreID || !snake.Alive {
			continue
		}
		for _, cell := range snake.Body {
			if cell == target {
				return true
			}
		}
	}
	return false
}

func (g *Game) hasFood(target Point) bool {
	for _, cell := range g.food {
		if cell == target {
			return true
		}
	}
	return false
}

func (g *Game) takeFood(target Point) bool {
	for i, cell := range g.food {
		if cell == target {
			g.food = append(g.food[:i], g.food[i+1:]...)
			return true
		}
	}
	return false
}

// refillFood 把食物补齐到配置数量，棋盘占满时提前结束。
func (g *Game) refillFood() {
	for len(g.food) < g.foodCount {
		if !g.spawnFood() {
			return
		}
	}
}

func (g *Game) spawnFood() bool {
	free := make([]Point, 0, g.width*g.height)
	for y := 0; y < g.height; y++ {
		for x := 0; x < g.width; x++ {
			cell := Point{X: x, Y: y}
			if !g.occupied(cell, "") && !g.hasFood(cell) {
				free = append(free, cell)
			}
		}
	}
	if len(free) == 0 {
		return false
	}
	g.food = append(g.food, free[g.rng.Intn(len(free))])
	return true
}

// freeSpawn 按随机顺序寻找能放下新蛇的位置。
func (g *Game) freeSpawn() (Point, Direction, bool) {
	dirs := []Direction{DirUp, DirDown, DirLeft, DirRight}
	for _, cell := range g.shuffledCells() {
		for _, dir := range dirs {
			if g.canPlace(cell, dir) {
				return cell, dir, true
			}
		}
	}
	return Point{}, DirUp, false
}

// canPlace 判断以 head 为头、dir 为朝向放置新蛇是否合法：
// 蛇身必须全部在棋盘内，且不与存活蛇或已有食物重叠。
func (g *Game) canPlace(head Point, dir Direction) bool {
	dx, dy := dir.Delta()
	for i := 0; i < initialLength; i++ {
		cell := Point{X: head.X - dx*i, Y: head.Y - dy*i}
		if !g.InBounds(cell) || g.occupied(cell, "") || g.hasFood(cell) {
			return false
		}
	}
	return true
}

// shuffledCells 返回棋盘全部格子的随机排列。
func (g *Game) shuffledCells() []Point {
	cells := make([]Point, 0, g.width*g.height)
	for y := 0; y < g.height; y++ {
		for x := 0; x < g.width; x++ {
			cells = append(cells, Point{X: x, Y: y})
		}
	}
	g.rng.Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })
	return cells
}
