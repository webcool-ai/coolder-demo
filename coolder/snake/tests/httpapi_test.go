package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/snake/src/adapters/httpapi"
	"example.com/snake/src/application"
	"example.com/snake/src/core"
)

// handlerEnv 直接驱动 HTTP 处理器而不绑定任何网络端口，
// 使接口行为在没有回环端口权限的环境中也完全可测。
type handlerEnv struct {
	t       *testing.T
	handler http.Handler
}

func newHandlerEnv(t *testing.T) *handlerEnv {
	t.Helper()
	hub := application.NewHub()
	handler, err := httpapi.NewServer(hub)
	if err != nil {
		hub.Close()
		t.Fatalf("创建 HTTP 服务失败: %v", err)
	}
	t.Cleanup(hub.Close)
	return &handlerEnv{t: t, handler: handler}
}

// do 发起一次请求并返回状态码、响应体与响应头。
func (e *handlerEnv) do(method, path, body string) (int, []byte, http.Header) {
	e.t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	e.handler.ServeHTTP(recorder, req)
	resp := recorder.Result()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("读取 %s %s 响应失败: %v", method, path, err)
	}
	return resp.StatusCode, data, resp.Header
}

func (e *handlerEnv) get(path string) (int, []byte) {
	e.t.Helper()
	status, data, _ := e.do(http.MethodGet, path, "")
	return status, data
}

func (e *handlerEnv) post(path string, payload any) (int, []byte) {
	e.t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		e.t.Fatalf("序列化 %s 请求失败: %v", path, err)
	}
	status, data, _ := e.do(http.MethodPost, path, string(body))
	return status, data
}

func (e *handlerEnv) postRaw(path, body string) (int, []byte) {
	e.t.Helper()
	status, data, _ := e.do(http.MethodPost, path, body)
	return status, data
}

func onlySnake(t *testing.T, view core.View) core.SnakeView {
	t.Helper()
	if len(view.Snakes) != 1 {
		t.Fatalf("期望房间里有一条蛇，实际 %d", len(view.Snakes))
	}
	return view.Snakes[0]
}

func decodeView(t *testing.T, data []byte) core.View {
	t.Helper()
	var view core.View
	if err := json.Unmarshal(data, &view); err != nil {
		t.Fatalf("响应不是合法的房间状态 JSON: %v（响应 %s）", err, data)
	}
	return view
}

func TestHTTPJoinConfigureAndInput(t *testing.T) {
	env := newHandlerEnv(t)

	status, data := env.post("/api/join", map[string]any{"room": "r1", "name": "甲"})
	if status != http.StatusOK {
		t.Fatalf("加入房间返回 %d: %s", status, data)
	}
	var joined struct {
		PlayerID string    `json:"playerId"`
		Room     string    `json:"room"`
		State    core.View `json:"state"`
	}
	if err := json.Unmarshal(data, &joined); err != nil {
		t.Fatalf("解析加入结果失败: %v", err)
	}
	if joined.PlayerID == "" || joined.Room != "r1" {
		t.Fatalf("加入结果不完整: %+v", joined)
	}
	if joined.State.Width != core.DefaultWidth || joined.State.Height != core.DefaultHeight {
		t.Fatalf("棋盘尺寸不正确: %dx%d", joined.State.Width, joined.State.Height)
	}
	if len(joined.State.Snakes) != 1 {
		t.Fatalf("加入后应有一条蛇，实际 %d", len(joined.State.Snakes))
	}

	status, data = env.post("/api/config", map[string]any{"playerId": joined.PlayerID, "foodCount": 8})
	if status != http.StatusOK {
		t.Fatalf("修改食物数量返回 %d: %s", status, data)
	}
	view := decodeView(t, data)
	if view.FoodCount != 8 || len(view.Food) != 8 {
		t.Fatalf("食物数量应为 8，实际 %d（格子 %d）", view.FoodCount, len(view.Food))
	}

	status, data = env.post("/api/config", map[string]any{"playerId": joined.PlayerID, "speed": "fast"})
	if status != http.StatusOK {
		t.Fatalf("设置速度返回 %d: %s", status, data)
	}
	if speed := onlySnake(t, decodeView(t, data)).Speed; speed != string(core.SpeedFast) {
		t.Fatalf("速度应为 fast，实际 %s", speed)
	}

	status, data = env.post("/api/config", map[string]any{"playerId": joined.PlayerID, "speed": "warp"})
	if status != http.StatusBadRequest || !strings.Contains(string(data), "error") {
		t.Fatalf("非法速度应返回带错误信息的 400，实际 %d: %s", status, data)
	}

	status, data = env.post("/api/input", map[string]any{"playerId": joined.PlayerID, "key": "space"})
	if status != http.StatusOK {
		t.Fatalf("空格输入返回 %d: %s", status, data)
	}
	if !onlySnake(t, decodeView(t, data)).Paused {
		t.Fatal("按空格后应处于暂停状态")
	}

	status, data = env.get("/api/rooms")
	if status != http.StatusOK {
		t.Fatalf("房间列表返回 %d: %s", status, data)
	}
	var rooms struct {
		Rooms []application.RoomInfo `json:"rooms"`
	}
	if err := json.Unmarshal(data, &rooms); err != nil {
		t.Fatalf("解析房间列表失败: %v", err)
	}
	if len(rooms.Rooms) != 1 || rooms.Rooms[0].Room != "r1" || rooms.Rooms[0].Players != 1 || rooms.Rooms[0].FoodCount != 8 {
		t.Fatalf("房间列表内容不正确: %+v", rooms.Rooms)
	}

	status, _ = env.post("/api/input", map[string]any{"playerId": "missing", "key": "space"})
	if status != http.StatusNotFound {
		t.Fatalf("未知玩家应返回 404，实际 %d", status)
	}

	status, data = env.postRaw("/api/join", "{not-json")
	if status != http.StatusBadRequest || !strings.Contains(string(data), "error") {
		t.Fatalf("非法 JSON 应返回带错误信息的 400，实际 %d: %s", status, data)
	}
}

func TestHTTPRoutingErrors(t *testing.T) {
	env := newHandlerEnv(t)

	if status, _, _ := env.do(http.MethodGet, "/api/join", ""); status != http.StatusMethodNotAllowed {
		t.Fatalf("以 GET 调用 /api/join 应返回 405，实际 %d", status)
	}
	if status, data := env.get("/api/nope"); status != http.StatusNotFound || !strings.Contains(string(data), "error") {
		t.Fatalf("未知接口应返回带错误信息的 404，实际 %d: %s", status, data)
	}
	if status, data := env.get("/api/stream?playerId=missing"); status != http.StatusNotFound {
		t.Fatalf("未知玩家订阅应返回 404，实际 %d: %s", status, data)
	}
}

func TestHTTPStaticFilesAndStateStream(t *testing.T) {
	env := newHandlerEnv(t)

	status, data := env.get("/")
	if status != http.StatusOK || !strings.Contains(string(data), "food-slider") {
		t.Fatalf("首页应包含食物拉杆，实际 %d: %s", status, data)
	}

	status, data = env.get("/app.js")
	if status != http.StatusOK || !strings.Contains(string(data), "/api/join") {
		t.Fatalf("前端脚本应包含接口调用，实际 %d: %s", status, data)
	}

	_, joinData := env.post("/api/join", map[string]any{"room": "直播房", "name": "甲"})
	var joined struct {
		PlayerID string `json:"playerId"`
	}
	if err := json.Unmarshal(joinData, &joined); err != nil {
		t.Fatalf("解析加入结果失败: %v", err)
	}
	if joined.PlayerID == "" {
		t.Fatal("加入结果缺少玩家标识")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/stream?playerId="+joined.PlayerID, nil).WithContext(ctx)
	stream := newStreamWriter()
	done := make(chan struct{})
	go func() {
		defer close(done)
		env.handler.ServeHTTP(stream, req)
	}()

	raw := waitForFrame(t, stream, "data: ")
	if contentType := stream.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("状态流 Content-Type 应为 text/event-stream，实际 %q", contentType)
	}
	if status := stream.statusCode(); status != http.StatusOK {
		t.Fatalf("状态流返回 %d", status)
	}
	view := firstStateFrame(t, raw)
	if view.Room != "直播房" || len(view.Snakes) != 1 {
		t.Fatalf("状态流首帧内容不正确: %+v", view)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("客户端断开后状态流没有退出")
	}
}

// streamWriter 是并发安全的流式响应写入器，用于代替会绑定端口的 httptest 服务器。
type streamWriter struct {
	mu      sync.Mutex
	header  http.Header
	status  int
	body    bytes.Buffer
	flushed chan struct{}
}

func newStreamWriter() *streamWriter {
	return &streamWriter{header: make(http.Header), flushed: make(chan struct{}, 1)}
}

func (w *streamWriter) Header() http.Header { return w.header }

func (w *streamWriter) WriteHeader(status int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == 0 {
		w.status = status
	}
}

func (w *streamWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

// Flush 实现 http.Flusher，用通道通知测试已有新数据可读。
func (w *streamWriter) Flush() {
	select {
	case w.flushed <- struct{}{}:
	default:
	}
}

func (w *streamWriter) statusCode() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *streamWriter) bodyText() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}

// waitForFrame 等待响应中出现标记，超过截止时间即判定失败。
func waitForFrame(t *testing.T, stream *streamWriter, marker string) string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if body := stream.bodyText(); strings.Contains(body, marker) {
			return body
		}
		select {
		case <-stream.flushed:
		case <-deadline:
			t.Fatalf("等待 %q 数据帧超时，已收到：%s", marker, stream.bodyText())
		}
	}
}

func firstStateFrame(t *testing.T, raw string) core.View {
	t.Helper()
	for _, line := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var view core.View
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &view); err != nil {
			t.Fatalf("状态帧不是合法 JSON: %v", err)
		}
		return view
	}
	t.Fatalf("状态流未下发任何状态帧: %s", raw)
	return core.View{}
}
