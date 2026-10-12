// Command server is the backend of the multiplayer snake game. It
// serves the static frontend from ./web and hosts every game room over
// WebSocket connections.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"

	"example.com/snake/src/game"
	"example.com/snake/src/ws"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	webDir := flag.String("web", "web", "static file directory")
	flag.Parse()

	mgr := game.NewManager(game.GridW, game.GridH, game.DefaultFood)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWS(mgr, w, r)
	})
	mux.HandleFunc("/api/rooms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(mgr.List())
	})
	mux.Handle("/", http.FileServer(http.Dir(*webDir)))

	log.Printf("多人贪吃蛇服务器已启动: http://localhost%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// serveWS upgrades the request and hands the connection to the manager.
func serveWS(mgr *game.Manager, w http.ResponseWriter, r *http.Request) {
	room := truncateRunes(strings.TrimSpace(r.URL.Query().Get("room")), 24)
	if room == "" {
		room = "room1"
	}
	name := truncateRunes(strings.TrimSpace(r.URL.Query().Get("name")), 12)
	speed := r.URL.Query().Get("speed")

	conn, err := ws.Upgrade(w, r)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
		return
	}
	mgr.Serve(conn, room, name, speed)
}

func truncateRunes(s string, n int) string {
	if runes := []rune(s); len(runes) > n {
		return string(runes[:n])
	}
	return s
}
