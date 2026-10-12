package game

import "time"

// Speed selects how fast a snake travels on the grid.
type Speed string

// The three supported speed settings.
const (
	SpeedSlow   Speed = "slow"
	SpeedMedium Speed = "medium"
	SpeedFast   Speed = "fast"
)

// SpeedInterval maps a speed setting to the time between two steps.
func SpeedInterval(s Speed) time.Duration {
	switch s {
	case SpeedSlow:
		return 250 * time.Millisecond
	case SpeedFast:
		return 110 * time.Millisecond
	default:
		return 165 * time.Millisecond
	}
}

// NormalizeSpeed maps unknown values to the medium speed.
func NormalizeSpeed(s string) Speed {
	switch Speed(s) {
	case SpeedSlow, SpeedFast:
		return Speed(s)
	default:
		return SpeedMedium
	}
}

// palette of player colors, assigned by join order.
var palette = []string{
	"#4ade80", "#60a5fa", "#f472b6", "#fbbf24",
	"#f97316", "#a78bfa", "#2dd4bf", "#f87171",
	"#c084fc", "#84cc16",
}

func colorFor(joinIndex int) string {
	return palette[(joinIndex-1+len(palette))%len(palette)]
}

// Snake is one player's snake inside a room. Body[0] is the head.
type Snake struct {
	ID    string
	Name  string
	Color string

	Body    []Point
	Dir     Point
	Pending []Point // queued direction changes, at most 2

	Speed Speed

	Alive  bool
	Paused bool
	Score  int
	Grow   int // cells the snake still has to grow

	next      time.Time // earliest time of the next step (server bookkeeping)
	deadTicks int       // ticks since death; corpses fade out
}
