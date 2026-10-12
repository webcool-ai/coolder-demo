package game

import (
	"testing"
	"time"
)

func testRoom(w, h int) *Room {
	return &Room{
		name:    "test",
		w:       w,
		h:       h,
		snakes:  make(map[string]*Snake),
		clients: make(map[*Client]struct{}),
	}
}

func TestWallKill(t *testing.T) {
	r := testRoom(20, 10)
	s := &Snake{
		ID: "p1", Name: "a", Color: "#fff", Speed: SpeedFast,
		Dir: Point{X: 1, Y: 0}, Alive: true,
		Body: []Point{{X: r.w - 1, Y: 3}},
	}
	r.snakes[s.ID] = s
	r.order = []string{s.ID}

	r.stepSnake(s, time.Now())

	if s.Alive {
		t.Fatal("snake hitting the wall must die")
	}
}

func TestEatFoodGrows(t *testing.T) {
	r := testRoom(20, 10)
	s := &Snake{
		ID: "p1", Name: "a", Color: "#fff", Speed: SpeedFast,
		Dir: Point{X: 1, Y: 0}, Alive: true,
		Body: []Point{{X: 5, Y: 5}, {X: 4, Y: 5}},
	}
	r.snakes[s.ID] = s
	r.order = []string{s.ID}
	r.food = []Point{{X: 6, Y: 5}}

	r.stepSnake(s, time.Now())

	if s.Score != 1 {
		t.Fatalf("score = %d, want 1", s.Score)
	}
	if len(s.Body) != 3 {
		t.Fatalf("body length = %d, want 3 (growing)", len(s.Body))
	}
	if len(r.food) != 0 {
		t.Fatalf("food left = %d, want 0", len(r.food))
	}
}

func TestReverseDirectionIgnored(t *testing.T) {
	r := testRoom(20, 10)
	s := &Snake{
		ID: "p1", Name: "a", Color: "#fff", Speed: SpeedFast,
		Dir: Point{X: 1, Y: 0}, Alive: true,
		Body:    []Point{{X: 5, Y: 5}, {X: 4, Y: 5}},
		Pending: []Point{{X: -1, Y: 0}},
	}
	r.snakes[s.ID] = s
	r.order = []string{s.ID}

	r.stepSnake(s, time.Now())

	if s.Dir != (Point{X: 1, Y: 0}) {
		t.Fatalf("dir = %v, want unchanged (180-degree turn rejected)", s.Dir)
	}
}

func TestFoodCountAdjust(t *testing.T) {
	r := testRoom(10, 10)
	r.foodCnt = 8
	r.maintainFood()
	if len(r.food) != 8 {
		t.Fatalf("food = %d, want 8", len(r.food))
	}
	r.foodCnt = 3
	r.maintainFood()
	if len(r.food) != 3 {
		t.Fatalf("food = %d, want 3", len(r.food))
	}
}

func TestSpeedIntervals(t *testing.T) {
	cases := map[Speed]time.Duration{
		SpeedSlow:   250 * time.Millisecond,
		SpeedMedium: 165 * time.Millisecond,
		SpeedFast:   110 * time.Millisecond,
	}
	for speed, want := range cases {
		if got := SpeedInterval(speed); got != want {
			t.Fatalf("SpeedInterval(%s) = %v, want %v", speed, got, want)
		}
	}
	if got := NormalizeSpeed("nope"); got != SpeedMedium {
		t.Fatalf("NormalizeSpeed(unknown) = %s, want medium", got)
	}
}
