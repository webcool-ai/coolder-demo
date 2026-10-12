package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
)

// 静态资源内嵌进二进制，部署只需一个可执行文件。
//
//go:embed static
var staticFS embed.FS

func main() {
	addr := flag.String("addr", ":8080", "HTTP 监听地址，例如 :8080")
	flag.Parse()

	staticRoot, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	mgr := NewManager()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", mgr.serveWS)         // 多人实时对战入口
	mux.HandleFunc("/api/rooms", mgr.handleRooms) // 大厅房间列表
	mux.Handle("/", http.FileServer(http.FS(staticRoot)))

	log.Printf("🐍 多人贪吃蛇服务器已启动：http://localhost%s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
