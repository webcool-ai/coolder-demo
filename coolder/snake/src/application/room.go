package application

import (
	"time"

	"example.com/snake/src/core"
)

// broadcastInterval 是向客户端推送状态的最小间隔。
const broadcastInterval = 50 * time.Millisecond

// tickPerBroadcast 表示每多少个领域 tick 广播一次状态。
const tickPerBroadcast = int(broadcastInterval / core.TickDuration)

// Player 是房间内的一个玩家。
type Player struct {
	ID       string
	Name     string
	room     *Room
	updates  chan []byte
	lastSeen time.Time
}

// Room 维护一个独立棋盘的房间与其中的玩家。
type Room struct {
	name    string
	game    *core.Game
	players map[string]*Player
	stop    chan struct{}
}

// runRoom 按固定节拍推进房间内的游戏并周期性广播状态。
func (h *Hub) runRoom(room *Room) {
	defer h.wg.Done()
	ticker := time.NewTicker(core.TickDuration)
	defer ticker.Stop()
	ticks := 0
	for {
		select {
		case <-h.done:
			return
		case <-room.stop:
			return
		case <-ticker.C:
			ticks++
			if h.stepRoom(room, ticks%tickPerBroadcast == 0) {
				return
			}
		}
	}
}

// stepRoom 在锁内推进一次游戏，needPublish 为真时广播状态；
// 返回 true 表示 hub 已关闭，房间循环应当退出。
func (h *Hub) stepRoom(room *Room, needPublish bool) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return true
	}
	room.game.Step()
	if needPublish {
		h.publishLocked(room)
	}
	return false
}
