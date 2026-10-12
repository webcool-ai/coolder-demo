// Package application 负责房间编排、游戏节拍与状态广播。
package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"example.com/snake/src/core"
)

const (
	maxRoomRunes    = 16
	maxNameRunes    = 12
	playerTimeout   = 30 * time.Second
	sweepInterval   = 5 * time.Second
	defaultRoomName = "lobby"
	defaultName     = "玩家"
)

// ErrClosed 表示服务正在关闭。
var ErrClosed = errors.New("服务已关闭")

// ErrRoomFull 表示房间棋盘已无空位。
var ErrRoomFull = errors.New("房间已满，无法再容纳新玩家")

// Config 描述一次在线配置修改，零值字段表示不修改。
type Config struct {
	FoodCount *int
	Speed     string
}

// RoomInfo 描述房间列表中的一项。
type RoomInfo struct {
	Room      string `json:"room"`
	Players   int    `json:"players"`
	FoodCount int    `json:"foodCount"`
}

// Hub 管理所有房间与玩家。
type Hub struct {
	mu      sync.Mutex
	rooms   map[string]*Room
	players map[string]*Player
	seed    int64
	next    int
	closed  bool
	done    chan struct{}
	wg      sync.WaitGroup
}

// NewHub 创建并启动房间编排器。
func NewHub() *Hub {
	hub := &Hub{
		rooms:   make(map[string]*Room),
		players: make(map[string]*Player),
		seed:    time.Now().UnixNano(),
		done:    make(chan struct{}),
	}
	hub.wg.Add(1)
	go hub.sweepLoop()
	return hub
}

// Join 让玩家以昵称加入（必要时创建）房间，返回玩家标识与该房间最新状态。
func (h *Hub) Join(roomName, playerName string) (string, core.View, error) {
	roomName = normalize(roomName, defaultRoomName, maxRoomRunes)
	playerName = normalize(playerName, defaultName, maxNameRunes)

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return "", core.View{}, ErrClosed
	}
	room, ok := h.rooms[roomName]
	if !ok {
		room = h.createRoomLocked(roomName)
	}
	h.next++
	playerID := fmt.Sprintf("p%d", h.next)
	if room.game.AddSnake(playerID, playerName) == nil {
		return "", core.View{}, ErrRoomFull
	}
	player := &Player{
		ID:       playerID,
		Name:     playerName,
		room:     room,
		updates:  make(chan []byte, 1),
		lastSeen: time.Now(),
	}
	room.players[playerID] = player
	h.players[playerID] = player
	return playerID, room.game.Snapshot(roomName), nil
}

// createRoomLocked 创建房间并启动其 tick 循环，调用方必须持有锁。
func (h *Hub) createRoomLocked(name string) *Room {
	h.seed++
	room := &Room{
		name:    name,
		game:    core.NewGame(core.DefaultWidth, core.DefaultHeight, h.seed),
		players: make(map[string]*Player),
		stop:    make(chan struct{}),
	}
	h.rooms[name] = room
	h.wg.Add(1)
	go h.runRoom(room)
	return room
}

// Snapshot 返回指定玩家所在房间的最新状态。
func (h *Hub) Snapshot(playerID string) (core.View, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	player, ok := h.players[playerID]
	if !ok {
		return core.View{}, core.ErrNoSuchPlayer
	}
	player.lastSeen = time.Now()
	return player.room.game.Snapshot(player.room.name), nil
}

// Input 处理玩家的一次按键（方向或空格）。
func (h *Hub) Input(playerID, key string) (core.View, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	player, ok := h.players[playerID]
	if !ok {
		return core.View{}, core.ErrNoSuchPlayer
	}
	player.lastSeen = time.Now()
	if err := player.room.game.Input(playerID, key); err != nil {
		return core.View{}, err
	}
	view := player.room.game.Snapshot(player.room.name)
	h.publishLocked(player.room)
	return view, nil
}

// Configure 修改房间食物数量或该玩家的速度档位。
func (h *Hub) Configure(playerID string, cfg Config) (core.View, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	player, ok := h.players[playerID]
	if !ok {
		return core.View{}, core.ErrNoSuchPlayer
	}
	player.lastSeen = time.Now()
	if cfg.Speed != "" {
		if _, ok := core.ParseSpeed(cfg.Speed); !ok {
			return core.View{}, core.ErrBadSpeed
		}
	}
	if cfg.FoodCount != nil {
		player.room.game.SetFoodCount(*cfg.FoodCount)
	}
	if cfg.Speed != "" {
		if _, err := player.room.game.SetSpeed(playerID, cfg.Speed); err != nil {
			return core.View{}, err
		}
	}
	view := player.room.game.Snapshot(player.room.name)
	h.publishLocked(player.room)
	return view, nil
}

// Subscribe 返回该玩家接收状态推送的通道。
func (h *Hub) Subscribe(playerID string) (<-chan []byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	player, ok := h.players[playerID]
	if !ok {
		return nil, core.ErrNoSuchPlayer
	}
	player.lastSeen = time.Now()
	payload, err := json.Marshal(player.room.game.Snapshot(player.room.name))
	if err != nil {
		return nil, err
	}
	select {
	case player.updates <- payload:
	default:
	}
	return player.updates, nil
}

// Touch 刷新玩家活跃时间，避免长连接被回收。
func (h *Hub) Touch(playerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if player, ok := h.players[playerID]; ok {
		player.lastSeen = time.Now()
	}
}

// Rooms 返回当前房间列表，按房间名排序。
func (h *Hub) Rooms() []RoomInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	infos := make([]RoomInfo, 0, len(h.rooms))
	for name, room := range h.rooms {
		infos = append(infos, RoomInfo{
			Room:      name,
			Players:   len(room.players),
			FoodCount: room.game.FoodCount(),
		})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Room < infos[j].Room })
	return infos
}

// Close 停止所有房间循环与后台清理。
func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	close(h.done)
	for name, room := range h.rooms {
		close(room.stop)
		delete(h.rooms, name)
	}
	for id := range h.players {
		delete(h.players, id)
	}
	h.mu.Unlock()
	h.wg.Wait()
}

// publishLocked 把房间最新状态推入各玩家的推送通道。
func (h *Hub) publishLocked(room *Room) {
	if len(room.players) == 0 {
		return
	}
	payload, err := json.Marshal(room.game.Snapshot(room.name))
	if err != nil {
		return
	}
	for _, player := range room.players {
		select {
		case player.updates <- payload:
		default:
		}
	}
}

func (h *Hub) sweepLoop() {
	defer h.wg.Done()
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-h.done:
			return
		case <-ticker.C:
			h.sweep()
		}
	}
}

// sweep 回收超时未活动的玩家，并移除已经空掉的房间。
func (h *Hub) sweep() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	cutoff := time.Now().Add(-playerTimeout)
	for id, player := range h.players {
		if player.lastSeen.After(cutoff) {
			continue
		}
		player.room.game.RemoveSnake(id)
		delete(player.room.players, id)
		delete(h.players, id)
	}
	for name, room := range h.rooms {
		if len(room.players) > 0 {
			continue
		}
		close(room.stop)
		delete(h.rooms, name)
	}
}

// normalize 规范房间名或昵称：去空格、空值取默认值、超长截断。
func normalize(value, fallback string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if utf8.RuneCountInString(value) > maxRunes {
		runes := []rune(value)
		value = string(runes[:maxRunes])
	}
	return value
}
