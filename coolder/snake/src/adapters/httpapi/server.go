// Package httpapi 把房间编排暴露为 HTTP + SSE 接口，并提供内嵌的前端页面。
package httpapi

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"example.com/snake/src/application"
	"example.com/snake/src/core"
)

//go:embed web
var webFiles embed.FS

// maxRequestBody 限制请求体大小。
const maxRequestBody = 4 << 10

// keepAliveInterval 是 SSE 保活间隔，同时刷新玩家活跃时间。
const keepAliveInterval = 15 * time.Second

// Server 提供贪吃蛇的 HTTP 接口与静态页面。
type Server struct {
	hub *application.Hub
	web http.Handler
}

// NewServer 创建 HTTP 处理器。
func NewServer(hub *application.Hub) (*Server, error) {
	sub, err := fs.Sub(webFiles, "web")
	if err != nil {
		return nil, fmt.Errorf("读取内嵌前端资源失败: %w", err)
	}
	return &Server{hub: hub, web: http.FileServer(http.FS(sub))}, nil
}

// ServeHTTP 分发 API 与静态资源请求。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		s.serveAPI(w, r)
		return
	}
	s.web.ServeHTTP(w, r)
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/join":
		s.handleJoin(w, r)
	case "/api/rooms":
		s.handleRooms(w, r)
	case "/api/input":
		s.handleInput(w, r)
	case "/api/config":
		s.handleConfig(w, r)
	case "/api/stream":
		s.handleStream(w, r)
	default:
		writeError(w, http.StatusNotFound, "未知接口")
	}
}

type joinRequest struct {
	Room string `json:"room"`
	Name string `json:"name"`
}

type joinResponse struct {
	PlayerID string    `json:"playerId"`
	Room     string    `json:"room"`
	State    core.View `json:"state"`
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req joinRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	playerID, view, err := s.hub.Join(req.Room, req.Name)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, joinResponse{PlayerID: playerID, Room: view.Room, State: view})
}

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string][]application.RoomInfo{"rooms": s.hub.Rooms()})
}

type inputRequest struct {
	PlayerID string `json:"playerId"`
	Key      string `json:"key"`
}

func (s *Server) handleInput(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req inputRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	view, err := s.hub.Input(req.PlayerID, req.Key)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type configRequest struct {
	PlayerID  string `json:"playerId"`
	FoodCount *int   `json:"foodCount"`
	Speed     string `json:"speed"`
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req configRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	view, err := s.hub.Configure(req.PlayerID, application.Config{FoodCount: req.FoodCount, Speed: req.Speed})
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handleStream 通过 SSE 持续下发房间状态。
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	playerID := r.URL.Query().Get("playerId")
	updates, err := s.hub.Subscribe(playerID)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "当前服务器不支持流式推送")
		return
	}
	header := w.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.hub.Touch(playerID)
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case payload, open := <-updates:
			if !open {
				return
			}
			if _, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		writeError(w, http.StatusMethodNotAllowed, "请求方法不被支持")
		return false
	}
	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	body := http.MaxBytesReader(w, r.Body, maxRequestBody)
	defer body.Close()
	if err := json.NewDecoder(body).Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// statusFor 把领域错误映射为 HTTP 状态码。
func statusFor(err error) int {
	switch {
	case errors.Is(err, core.ErrNoSuchPlayer):
		return http.StatusNotFound
	case errors.Is(err, core.ErrBadSpeed), errors.Is(err, core.ErrBadKey):
		return http.StatusBadRequest
	case errors.Is(err, application.ErrClosed):
		return http.StatusServiceUnavailable
	case errors.Is(err, application.ErrRoomFull):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
