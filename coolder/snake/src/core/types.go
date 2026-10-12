// Package core 提供贪吃蛇的纯领域逻辑：棋盘、蛇的移动与食物规则。
// 该包不依赖网络、并发或渲染，便于单元测试。
package core

import (
	"errors"
	"time"
)

// 棋盘默认尺寸与食物数量范围。
const (
	DefaultWidth  = 40
	DefaultHeight = 26

	MinFoodCount     = 1
	MaxFoodCount     = 20
	DefaultFoodCount = 5
)

// TickDuration 是领域时钟的最小步长，速度档位以其整数倍决定移动频率。
const TickDuration = 10 * time.Millisecond

// initialLength 是新蛇的初始长度。
const initialLength = 3

// 领域错误。
var (
	ErrNoSuchPlayer = errors.New("玩家不存在")
	ErrBadSpeed     = errors.New("未知的速度档位")
	ErrBadKey       = errors.New("不支持的按键")
)

// Point 表示棋盘上的一个格子。
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Direction 表示蛇头朝向。
type Direction int

// 方向取值。
const (
	DirUp Direction = iota
	DirDown
	DirLeft
	DirRight
)

// Delta 返回方向对应的坐标增量。
func (d Direction) Delta() (int, int) {
	switch d {
	case DirUp:
		return 0, -1
	case DirDown:
		return 0, 1
	case DirLeft:
		return -1, 0
	default:
		return 1, 0
	}
}

// Opposite 判断另一方向是否与当前方向相反。
func (d Direction) Opposite(other Direction) bool {
	dx, dy := d.Delta()
	ox, oy := other.Delta()
	return dx == -ox && dy == -oy
}

// DirectionForKey 把按键名映射为方向。
func DirectionForKey(key string) (Direction, bool) {
	switch key {
	case "up":
		return DirUp, true
	case "down":
		return DirDown, true
	case "left":
		return DirLeft, true
	case "right":
		return DirRight, true
	default:
		return DirUp, false
	}
}

// Speed 是蛇的运行速度档位。
type Speed string

// 速度档位取值。
const (
	SpeedSlow   Speed = "slow"
	SpeedMedium Speed = "medium"
	SpeedFast   Speed = "fast"
)

// Interval 返回该档位下每移动一格所需的时间。
func (s Speed) Interval() time.Duration {
	switch s {
	case SpeedSlow:
		return 200 * time.Millisecond
	case SpeedFast:
		return 80 * time.Millisecond
	default:
		return 120 * time.Millisecond
	}
}

// ParseSpeed 解析速度档位字符串。
func ParseSpeed(value string) (Speed, bool) {
	switch Speed(value) {
	case SpeedSlow, SpeedMedium, SpeedFast:
		return Speed(value), true
	default:
		return SpeedMedium, false
	}
}
