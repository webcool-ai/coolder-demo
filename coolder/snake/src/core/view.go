package core

// SnakeView 是快照中的一条蛇。
type SnakeView struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Body   []Point `json:"body"`
	Alive  bool    `json:"alive"`
	Paused bool    `json:"paused"`
	Score  int     `json:"score"`
	Speed  string  `json:"speed"`
}

// View 是房间的完整状态快照，也是前端渲染的唯一数据来源。
type View struct {
	Room      string      `json:"room"`
	Width     int         `json:"width"`
	Height    int         `json:"height"`
	FoodCount int         `json:"foodCount"`
	Food      []Point     `json:"food"`
	Snakes    []SnakeView `json:"snakes"`
}
