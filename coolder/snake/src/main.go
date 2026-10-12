// Command snake 启动多人在线、多房间的贪吃蛇服务。
package main

import (
	"log"
	"net"
	"net/http"
	"os"

	"example.com/snake/src/adapters/httpapi"
	"example.com/snake/src/application"
)

// defaultPort 是未提供 WEBCOOL_HTTP_PORT 时使用的端口。
const defaultPort = "18080"

func main() {
	hub := application.NewHub()
	defer hub.Close()

	server, err := httpapi.NewServer(hub)
	if err != nil {
		log.Fatalf("初始化 HTTP 服务失败: %v", err)
	}
	addr := net.JoinHostPort("127.0.0.1", listenPort())
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("监听 %s 失败: %v", addr, err)
	}
	log.Printf("多人在线贪吃蛇已启动，浏览器打开 http://%s", listener.Addr().String())
	if err := http.Serve(listener, server); err != nil {
		log.Fatalf("服务异常退出: %v", err)
	}
}

// listenPort 读取运行环境注入的端口，未设置时回落到默认端口。
func listenPort() string {
	if port := os.Getenv("WEBCOOL_HTTP_PORT"); port != "" {
		return port
	}
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return defaultPort
}
