// Package game contains the authoritative multiplayer snake engine:
// rooms, snakes, food and the websocket client wiring.
package game

import "encoding/json"

// Point is a grid cell coordinate.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Client -> server message types.
const (
	MsgDir     = "dir"     // {type, dx, dy}
	MsgSpeed   = "speed"   // {type, speed}
	MsgFood    = "food"    // {type, food}
	MsgPause   = "pause"   // {type, paused}
	MsgRestart = "restart" // {type}
	MsgPing    = "ping"    // {type} keepalive
)

// ClientMsg is any message sent by a browser client.
type ClientMsg struct {
	Type   string `json:"type"`
	DX     int    `json:"dx,omitempty"`
	DY     int    `json:"dy,omitempty"`
	Speed  string `json:"speed,omitempty"`
	Food   int    `json:"food,omitempty"`
	Paused bool   `json:"paused,omitempty"`
}

// Server -> client message types.
const (
	MsgWelcome = "welcome"
	MsgState   = "state"
	MsgError   = "error"
)

// SnakeView is the wire representation of one snake.
type SnakeView struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Body   []Point `json:"body"`
	Dir    Point   `json:"dir"`
	Color  string  `json:"color"`
	Score  int     `json:"score"`
	Alive  bool    `json:"alive"`
	Paused bool    `json:"paused"`
	Speed  string  `json:"speed"`
}

// WelcomeMsg is sent once right after a client joins a room.
type WelcomeMsg struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Room      string `json:"room"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	FoodCount int    `json:"foodCount"`
	MaxFood   int    `json:"maxFood"`
	Speed     string `json:"speed"`
}

// StateMsg is the full room snapshot broadcast every tick.
type StateMsg struct {
	Type   string       `json:"type"`
	Tick   int          `json:"tick"`
	Food   []Point      `json:"food"`
	Snakes []*SnakeView `json:"snakes"`
}

// ErrorMsg reports a recoverable problem to a single client.
type ErrorMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // all messages are marshalable plain data
	}
	return b
}
