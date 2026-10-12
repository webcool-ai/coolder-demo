package core

import "testing"

func TestSetFoodCountClampsAndKeepsFoodOnBoard(t *testing.T) {
	game := NewGame(DefaultWidth, DefaultHeight, 11)
	if len(game.Food()) != DefaultFoodCount {
		t.Fatalf("默认食物数量应为 %d，实际 %d", DefaultFoodCount, len(game.Food()))
	}

	if got := game.SetFoodCount(12); got != 12 || len(game.Food()) != 12 {
		t.Fatalf("食物数量应变为 12，实际 %d（格子 %d）", got, len(game.Food()))
	}
	if got := game.SetFoodCount(-5); got != MinFoodCount || len(game.Food()) != MinFoodCount {
		t.Fatalf("食物数量应被压到下限 %d，实际 %d（格子 %d）", MinFoodCount, got, len(game.Food()))
	}
	if got := game.SetFoodCount(999); got != MaxFoodCount || len(game.Food()) != MaxFoodCount {
		t.Fatalf("食物数量应被压到上限 %d，实际 %d（格子 %d）", MaxFoodCount, got, len(game.Food()))
	}

	game.AddSnake("p1", "甲")
	game.SetFoodCount(MaxFoodCount)
	seen := make(map[Point]bool)
	for _, cell := range game.Food() {
		if !game.InBounds(cell) {
			t.Fatalf("食物越出棋盘: %+v", cell)
		}
		if seen[cell] {
			t.Fatalf("食物位置重复: %+v", cell)
		}
		seen[cell] = true
		if game.occupied(cell, "") {
			t.Fatalf("食物不应与蛇身重叠: %+v", cell)
		}
	}
}

func TestSpeedPresetIntervalsAndParsing(t *testing.T) {
	if !(SpeedSlow.Interval() > SpeedMedium.Interval() && SpeedMedium.Interval() > SpeedFast.Interval()) {
		t.Fatalf("慢/中/快三档间隔应依次递减: %v %v %v", SpeedSlow.Interval(), SpeedMedium.Interval(), SpeedFast.Interval())
	}
	cases := []struct {
		value string
		want  Speed
		ok    bool
	}{
		{"slow", SpeedSlow, true},
		{"medium", SpeedMedium, true},
		{"fast", SpeedFast, true},
		{"warp", SpeedMedium, false},
		{"", SpeedMedium, false},
	}
	for _, tc := range cases {
		got, ok := ParseSpeed(tc.value)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("ParseSpeed(%q) = (%v, %v)，期望 (%v, %v)", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

func TestSnakeMovesOnlyWhenSpeedIntervalElapsed(t *testing.T) {
	game := NewGame(20, 20, 5)
	snake := game.AddSnake("p1", "甲")
	if snake == nil {
		t.Fatal("应能在空棋盘上生成蛇")
	}
	snake.Body = []Point{{X: 5, Y: 5}, {X: 4, Y: 5}, {X: 3, Y: 5}}
	snake.Direction = DirRight
	snake.pending = DirRight
	snake.Speed = SpeedSlow

	game.Step()
	if snake.Body[0] != (Point{X: 5, Y: 5}) {
		t.Fatalf("速度间隔未到不应移动，实际 %+v", snake.Body[0])
	}

	for i := 0; i < int(SpeedSlow.Interval()/TickDuration); i++ {
		game.Step()
	}
	if snake.Body[0] != (Point{X: 6, Y: 5}) {
		t.Fatalf("慢速档应在 %v 后移动一格，实际 %+v", SpeedSlow.Interval(), snake.Body[0])
	}
	if len(snake.Body) != initialLength {
		t.Fatalf("未吃到食物时长度应保持 %d，实际 %d", initialLength, len(snake.Body))
	}
}

func TestEatFoodGrowsAndRefillsFoodCount(t *testing.T) {
	game := NewGame(20, 20, 7)
	snake := game.AddSnake("p1", "甲")
	snake.Body = []Point{{X: 5, Y: 5}, {X: 4, Y: 5}, {X: 3, Y: 5}}
	snake.Direction = DirRight
	snake.pending = DirRight
	snake.Speed = SpeedFast
	game.foodCount = 1
	game.food = []Point{{X: 6, Y: 5}}

	for i := 0; i < 50 && snake.Score == 0; i++ {
		game.Step()
	}
	if snake.Score != 10 {
		t.Fatalf("吃到食物应得 10 分，实际 %d", snake.Score)
	}
	if len(snake.Body) != initialLength+1 {
		t.Fatalf("吃到食物后长度应为 %d，实际 %d", initialLength+1, len(snake.Body))
	}
	food := game.Food()
	if len(food) != 1 {
		t.Fatalf("食物应补回配置数量 1，实际 %d", len(food))
	}
	if food[0] == (Point{X: 6, Y: 5}) {
		t.Fatalf("被吃掉的位置不应再次出现食物: %+v", food[0])
	}
}

func TestWallCollisionKillsAndSpaceRestarts(t *testing.T) {
	game := NewGame(12, 12, 3)
	snake := game.AddSnake("p1", "甲")
	snake.Body = []Point{{X: 10, Y: 5}, {X: 9, Y: 5}, {X: 8, Y: 5}}
	snake.Direction = DirRight
	snake.pending = DirRight
	snake.Speed = SpeedFast

	for i := 0; i < 200 && snake.Alive; i++ {
		game.Step()
	}
	if snake.Alive {
		t.Fatal("蛇头撞到边界后应死亡")
	}

	if err := game.Input("p1", "space"); err != nil {
		t.Fatalf("死亡后按空格应被接受: %v", err)
	}
	if !snake.Alive || snake.Paused || snake.Score != 0 {
		t.Fatalf("按空格应重新开始，实际 alive=%v paused=%v score=%d", snake.Alive, snake.Paused, snake.Score)
	}
	if len(snake.Body) != initialLength {
		t.Fatalf("重新开始后长度应为 %d，实际 %d", initialLength, len(snake.Body))
	}
	for _, cell := range snake.Body {
		if !game.InBounds(cell) {
			t.Fatalf("重新开始后蛇身越界: %+v", cell)
		}
	}
}

func TestSpaceTogglesPauseAndBlocksMovement(t *testing.T) {
	game := NewGame(20, 20, 9)
	snake := game.AddSnake("p1", "甲")
	snake.Body = []Point{{X: 5, Y: 5}, {X: 4, Y: 5}, {X: 3, Y: 5}}
	snake.Direction = DirRight
	snake.pending = DirRight
	snake.Speed = SpeedFast

	if err := game.Input("p1", "space"); err != nil {
		t.Fatalf("空格输入失败: %v", err)
	}
	if !snake.Paused {
		t.Fatal("按一次空格应进入暂停")
	}
	head := snake.Body[0]
	for i := 0; i < 100; i++ {
		game.Step()
	}
	if snake.Body[0] != head {
		t.Fatalf("暂停期间不应移动，实际 %+v", snake.Body[0])
	}

	if err := game.Input("p1", "space"); err != nil {
		t.Fatalf("再次按空格失败: %v", err)
	}
	if snake.Paused {
		t.Fatal("再按一次空格应继续游戏")
	}
	for i := 0; i < 20 && snake.Body[0] == head; i++ {
		game.Step()
	}
	if snake.Body[0] == head {
		t.Fatal("继续后蛇应开始移动")
	}
}

func TestInputIgnoresReverseDirectionAndUnknownKeys(t *testing.T) {
	game := NewGame(20, 20, 13)
	snake := game.AddSnake("p1", "甲")
	snake.Body = []Point{{X: 5, Y: 5}, {X: 4, Y: 5}, {X: 3, Y: 5}}
	snake.Direction = DirRight
	snake.pending = DirRight
	snake.Speed = SpeedFast

	if err := game.Input("p1", "left"); err != nil {
		t.Fatalf("反向输入不应报错: %v", err)
	}
	for i := 0; i < int(SpeedFast.Interval()/TickDuration); i++ {
		game.Step()
	}
	if snake.Body[0] != (Point{X: 6, Y: 5}) {
		t.Fatalf("反向输入应被忽略，实际 %+v", snake.Body[0])
	}

	if err := game.Input("p1", "warp"); err == nil {
		t.Fatal("未知按键应返回错误")
	}
	if err := game.Input("missing", "space"); err != ErrNoSuchPlayer {
		t.Fatalf("未知玩家应返回 ErrNoSuchPlayer，实际 %v", err)
	}
}

func TestSelfCollisionKillsSnake(t *testing.T) {
	game := NewGame(20, 20, 4)
	snake := game.AddSnake("p1", "甲")
	snake.Body = []Point{{X: 5, Y: 5}, {X: 5, Y: 6}, {X: 6, Y: 6}, {X: 6, Y: 5}, {X: 7, Y: 5}}
	snake.Direction = DirRight
	snake.pending = DirRight
	snake.Speed = SpeedFast

	for i := 0; i < 20 && snake.Alive; i++ {
		game.Step()
	}
	if snake.Alive {
		t.Fatal("蛇头撞到自身躯干后应死亡")
	}
}
