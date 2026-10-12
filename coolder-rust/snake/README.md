# 多人贪吃蛇

纯 Go 实现的多人在线贪吃蛇：后端权威模拟 + 浏览器 Canvas 渲染，
WebSocket 通信，全部基于标准库，无任何第三方依赖。

## 玩法

- 进入首页 → 输入房间号与玩家名 →「进入游戏」（同名房间即同一张地图）
- 方向键 / WASD 控制方向，空格暂停/继续，触屏可滑动转向
- 吃食物加分变长；撞墙或撞到任何蛇身即出局（尸体 1.2 秒后消散）
- 死亡后按「重新开始 (R)」原地复活，或点「观战」继续看别人打
- 侧栏可实时调整食物数量（0~40）与速度（慢/中/快，死亡后也可切速）

## 运行

需要 Go 1.21+。

```bash
go build -o build/snake ./src
./build/snake --addr :8080 --web web
```

或直接使用脚本：

- Linux / macOS：`./build.sh && ./run.sh`
- Windows：`build.bat && run.bat`

打开 <http://localhost:8080> 即可游戏；多开几个浏览器标签页即可联机。

## 架构

```
web/                 零依赖前端：Canvas 渲染、排行榜、设置同步
src/main.go          HTTP 入口：静态文件、/api/rooms、/ws 升级
src/ws/              仅标准库实现的 RFC 6455 WebSocket
                     （握手、文本帧、分片、ping/pong、close）
src/game/            房间与引擎
```

要点：

- 每个房间独立 goroutine，50ms 一拍；快/中/慢蛇按各自速度（110/165/250ms）走格
- 服务器完全权威：方向输入进入队列（最多缓存 2 个）、过滤 180° 折返，
  碰撞、食物、生长全部在服务端判定，客户端只做渲染
- 每拍广播全量状态快照；输出通道带缓冲 + 非阻塞发送，慢客户端只会丢帧，
  不会阻塞整个房间
- WS 帧写入带 10 秒超时，`Close` 直接关套接字解除阻塞，坏连接不会拖死房间
- 客户端每 25s ping 保活；服务器读空闲 90s 超时断开并清理
- 空闲房间 10 分钟后由 janitor 自动回收；加入中的房间受 `reserved` 标记保护

## 协议

- 客户端 → 服务器 JSON：`dir / speed / food / pause / restart / ping`
- 服务器 → 客户端：`welcome`（加入应答，含房间尺寸与个人 ID）、
  `state`（全量快照，每拍广播）、`error`
- 加入流程：upgrade → `welcome` → 持续 `state`，顺序由发送队列保证

## 测试

```bash
go test ./...
go vet ./...
```

覆盖：撞墙、吃食物生长、180° 折返过滤、食物数量调整、速度映射、
名字截断（含多字节字符），以及 WebSocket 握手与帧回环
（echo / ping→pong / close 握手闭环），无需浏览器即可验证通信层。
