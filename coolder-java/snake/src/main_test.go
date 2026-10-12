package main

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeSpeedAndInterval(t *testing.T) {
	if normalizeSpeed("fast") != "fast" {
		t.Fatal("fast 应该是合法速度")
	}
	if normalizeSpeed("???") != "normal" {
		t.Fatal("非法速度应回退到中速")
	}
	if speedInterval("slow") != 300*time.Millisecond {
		t.Fatal("慢速应为 300ms/帧")
	}
	if speedInterval("normal") != 160*time.Millisecond {
		t.Fatal("中速应为 160ms/帧")
	}
	if speedInterval("fast") != 90*time.Millisecond {
		t.Fatal("快速应为 90ms/帧")
	}
}

func TestClampFoodTarget(t *testing.T) {
	if clampFoodTarget(0) != MinFood {
		t.Fatal("低于下限应收敛到 MinFood")
	}
	if clampFoodTarget(9999) != MaxFood {
		t.Fatal("高于上限应收敛到 MaxFood")
	}
	if clampFoodTarget(7) != 7 {
		t.Fatal("正常值不应被改变")
	}
}

// 滑杆拉高/拉低后，食物数量应渐进逼近目标值。
func TestFoodFollowsSliderTarget(t *testing.T) {
	w := NewWorld()
	w.FoodTarget = 12
	for i := 0; i < 30 && len(w.Foods) < 12; i++ {
		w.Tick()
	}
	if len(w.Foods) != 12 {
		t.Fatalf("食物应增长到 12，实际 %d", len(w.Foods))
	}
	w.FoodTarget = 3
	for i := 0; i < 30 && len(w.Foods) > 3; i++ {
		w.Tick()
	}
	if len(w.Foods) != 3 {
		t.Fatalf("食物应减少到 3，实际 %d", len(w.Foods))
	}
}

func mkSnake(cells ...Cell) *Snake {
	return &Snake{ID: 1, Name: "测试蛇", Color: "#ffffff", Alive: true, Dir: DirRight, Cells: cells}
}

// 撞墙死亡：尸体保留原地，事件提示；按空格（RespawnSnake）重新开始。
func TestWallDeathAndRestartBySpace(t *testing.T) {
	w := NewWorld()
	w.FoodTarget = 0
	w.Foods = nil
	s := mkSnake(
		Cell{GridW - 1, 5}, Cell{GridW - 2, 5},
		Cell{GridW - 3, 5}, Cell{GridW - 4, 5},
	)
	w.Snakes = append(w.Snakes, s)

	events := w.Tick()
	if s.Alive {
		t.Fatal("蛇头出界（撞墙）后应该死亡")
	}
	if len(s.Cells) != 4 || s.Cells[0] != (Cell{GridW - 1, 5}) {
		t.Fatal("尸体应保持在死亡前的位置")
	}
	found := false
	for _, e := range events {
		if strings.Contains(e, "撞墙") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应有撞墙事件，实际 %v", events)
	}

	if !w.RespawnSnake(s) {
		t.Fatal("重新开始应成功")
	}
	if !s.Alive || s.Score != 0 || len(s.Cells) != StartLen {
		t.Fatal("重新开始后应满血复活且得分为 0")
	}
	h := s.Cells[0]
	if h.X < 0 || h.X >= GridW || h.Y < 0 || h.Y >= GridH {
		t.Fatal("重生位置应在棋盘内")
	}
}

// 吃到食物：变长、得分、食物消失。
func TestEatFoodGrows(t *testing.T) {
	w := NewWorld()
	w.FoodTarget = 0
	w.Foods = nil
	s := mkSnake(Cell{10, 10}, Cell{9, 10}, Cell{8, 10}, Cell{7, 10})
	w.Snakes = append(w.Snakes, s)
	w.Foods = append(w.Foods, Cell{11, 10})

	w.Tick()
	if len(s.Cells) != StartLen+1 {
		t.Fatalf("吃到食物后长度应为 %d，实际 %d", StartLen+1, len(s.Cells))
	}
	if s.Score != FoodPoints {
		t.Fatalf("得分应为 %d，实际 %d", FoodPoints, s.Score)
	}
	if w.foodAt(Cell{11, 10}) {
		t.Fatal("食物应被吃掉")
	}
}

// 空格暂停后，世界冻结。
func TestPauseFreezesWorld(t *testing.T) {
	w := NewWorld()
	w.Running = false // 等价于按了空格暂停
	s := mkSnake(Cell{10, 10}, Cell{9, 10}, Cell{8, 10}, Cell{7, 10})
	w.Snakes = append(w.Snakes, s)

	w.Tick()
	if s.Cells[0] != (Cell{10, 10}) {
		t.Fatal("暂停时蛇不应移动")
	}
}

// 方向队列校验：不能 180° 掉头；合法转向下一帧生效。
func TestEnqueueDirValidation(t *testing.T) {
	s := mkSnake(Cell{10, 10}, Cell{9, 10}, Cell{8, 10}, Cell{7, 10})
	if s.EnqueueDir(DirLeft) {
		t.Fatal("向右行进时不允许直接反向")
	}
	if !s.EnqueueDir(DirUp) {
		t.Fatal("合法转向应入队")
	}
	if s.EnqueueDir(DirDown) {
		t.Fatal("与队尾方向相反的输入应被拒绝")
	}

	w := NewWorld()
	w.Snakes = append(w.Snakes, s)
	w.Tick()
	if s.Dir != DirUp {
		t.Fatalf("下一帧应应用队列中的方向，实际 %v", s.Dir)
	}
	if s.Cells[0] != (Cell{10, 9}) {
		t.Fatalf("蛇头应向上移动一格，实际 %v", s.Cells[0])
	}
}

// 迎头相撞：同帧冲向同一格的两条蛇都死亡。
func TestHeadOnCollisionKillsBoth(t *testing.T) {
	w := NewWorld()
	w.FoodTarget = 0
	w.Foods = nil
	a := &Snake{ID: 1, Name: "A", Color: "#fff", Alive: true, Dir: DirRight,
		Cells: []Cell{{10, 5}, {9, 5}, {8, 5}, {7, 5}}}
	b := &Snake{ID: 2, Name: "B", Color: "#0f0", Alive: true, Dir: DirLeft,
		Cells: []Cell{{13, 5}, {14, 5}, {15, 5}, {16, 5}}}
	w.Snakes = append(w.Snakes, a, b)

	w.Tick()
	if a.Alive || b.Alive {
		t.Fatal("迎头相撞的两条蛇都应死亡")
	}
}
