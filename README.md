# coolder-demo

[中文](#中文) | [English](#english)

## 中文

本仓库收集由 [coolder 项目](https://github.com/webcool-ai/coolder)中的三个 AI coding APP —— **coolder**、**coolder-java** 和 **coolder-rust** —— 分别编写的示例程序，用于展示它们生成的应用，并方便运行、体验和对照阅读代码。

## 示例与目录

每个顶层目录对应编写其中示例的 AI coding APP。目前三个目录均包含一个 `snake` 示例：多人在线、多房间的贪吃蛇游戏。

| 目录 | 编写示例的 AI coding APP | 当前示例 | 详细说明 |
| --- | --- | --- | --- |
| [`coolder/`](./coolder/) | `coolder` | 多人在线贪吃蛇 | [运行与玩法](./coolder/snake/README.md) |
| [`coolder-java/`](./coolder-java/) | `coolder-java` | 多人在线贪吃蛇 | [运行、玩法与代码结构](./coolder-java/snake/README.md) |
| [`coolder-rust/`](./coolder-rust/) | `coolder-rust` | 多人在线贪吃蛇 | [运行、玩法与架构](./coolder-rust/snake/README.md) |

**目录名表示生成示例的 APP 名称，不代表示例的实现语言。** 当前三个贪吃蛇示例均采用 Go 后端与 HTML / CSS / JavaScript 浏览器前端，通过 WebSocket 同步游戏状态。

```text
coolder-demo/
├── coolder/
│   └── snake/        # 由 coolder 编写
├── coolder-java/
│   └── snake/        # 由 coolder-java 编写
├── coolder-rust/
│   └── snake/        # 由 coolder-rust 编写
├── LICENSE
└── README.md
```

## 游戏体验

三个示例都支持以下基本功能，具体规则和界面各有差异：

- 输入玩家昵称与房间名，加入房间进行多人游戏。
- 在浏览器中使用方向键或 WASD 控制贪吃蛇。
- 调整食物数量与游戏速度。
- 暂停、继续，以及死亡后重新开始。

可以打开多个浏览器标签页，使用不同昵称进入同一房间体验多人游戏，也可以进入不同房间观察房间隔离效果。食物范围、速度设置的作用范围、碰撞和复活规则请参阅各示例的 README。

## 快速开始

### 环境要求

- Go 1.21 或更高版本。
- 支持 WebSocket 与 Canvas 的现代浏览器。
- macOS / Linux 使用 `.sh` 脚本；Windows 使用对应的 `.bat` 脚本。

以下命令均从本仓库根目录开始执行，任选一个示例即可。

### coolder 编写的示例

```bash
cd coolder/snake
./build.sh
./run.sh
```

浏览器访问 <http://127.0.0.1:18080>。可通过环境变量修改端口，例如：

```bash
WEBCOOL_HTTP_PORT=18081 ./run.sh
```

该示例的构建脚本还会执行 `go test ./...`。

### coolder-java 编写的示例

```bash
cd coolder-java/snake
./build.sh
./run.sh
```

浏览器访问 <http://localhost:8080>。可使用 `./run.sh -addr :8081` 修改监听端口。

### coolder-rust 编写的示例

```bash
cd coolder-rust/snake
./build.sh
./run.sh
```

浏览器访问 <http://localhost:8080>。可使用 `./run.sh -addr :8082` 修改监听端口。请在 `coolder-rust/snake` 目录中启动，以便服务找到默认的 `web/` 静态资源目录。

在 Windows 下，进入相应的 `snake` 目录后依次执行 `build.bat` 和 `run.bat`。如果同时运行多个示例，请为它们设置不同端口，尤其是默认都使用 `8080` 的两个示例。

## 测试

三个示例都是独立的 Go 模块。在要检查的 `snake` 目录中执行：

```bash
go test ./...
```

## 相关项目

- [coolder](https://github.com/webcool-ai/coolder)：提供编写本仓库示例的 `coolder`、`coolder-java`、`coolder-rust` 三个 AI coding APP。

## 许可证

本仓库采用 [MIT License](./LICENSE)。

---

## English

This repository collects demo applications written by three AI coding apps from the [coolder project](https://github.com/webcool-ai/coolder): **coolder**, **coolder-java**, and **coolder-rust**. It showcases the applications they generate and makes it easy to run the demos, try them out, and compare their code.

### Demos and directories

Each top-level demo directory corresponds to the AI coding app that wrote its contents. All three currently contain a `snake` demo: an online multiplayer Snake game with multiple rooms.

| Directory | AI coding app that wrote the demo | Current demo | Documentation |
| --- | --- | --- | --- |
| [`coolder/`](./coolder/) | `coolder` | Online multiplayer Snake | [Running and gameplay](./coolder/snake/README.md) |
| [`coolder-java/`](./coolder-java/) | `coolder-java` | Online multiplayer Snake | [Running, gameplay, and code structure](./coolder-java/snake/README.md) |
| [`coolder-rust/`](./coolder-rust/) | `coolder-rust` | Online multiplayer Snake | [Running, gameplay, and architecture](./coolder-rust/snake/README.md) |

The linked demo documentation is currently in Chinese.

**Directory names identify the apps that generated the demos, not the programming languages used by the demos.** All three current Snake demos use a Go backend and an HTML / CSS / JavaScript browser frontend, with game state synchronized over WebSocket.

```text
coolder-demo/
├── coolder/
│   └── snake/        # Written by coolder
├── coolder-java/
│   └── snake/        # Written by coolder-java
├── coolder-rust/
│   └── snake/        # Written by coolder-rust
├── LICENSE
└── README.md
```

### Gameplay

All three demos support the following basic features, with differences in their rules and interfaces:

- Enter a player nickname and room name to join a multiplayer game.
- Control your snake in the browser using the arrow keys or WASD.
- Adjust the amount of food and the game speed.
- Pause, resume, and restart after dying.

Open multiple browser tabs and join the same room with different nicknames to try multiplayer gameplay. You can also join different rooms to see how games run independently. See each demo's README for food limits, whether speed settings apply to individual players or the entire room, and collision and respawn rules.

### Quick start

#### Requirements

- Go 1.21 or later.
- A modern browser with WebSocket and Canvas support.
- Use `.sh` scripts on macOS / Linux and the corresponding `.bat` scripts on Windows.

Each set of commands below starts from the repository root. Choose any one demo.

#### Demo written by coolder

```bash
cd coolder/snake
./build.sh
./run.sh
```

Open <http://127.0.0.1:18080> in your browser. To change the port, set the environment variable, for example:

```bash
WEBCOOL_HTTP_PORT=18081 ./run.sh
```

This demo's build script also runs `go test ./...`.

#### Demo written by coolder-java

```bash
cd coolder-java/snake
./build.sh
./run.sh
```

Open <http://localhost:8080> in your browser. Use `./run.sh -addr :8081` to change the listening port.

#### Demo written by coolder-rust

```bash
cd coolder-rust/snake
./build.sh
./run.sh
```

Open <http://localhost:8080> in your browser. Use `./run.sh -addr :8082` to change the listening port. Start the server from `coolder-rust/snake` so it can find the default `web/` static assets directory.

On Windows, enter the appropriate `snake` directory and run `build.bat`, followed by `run.bat`. When running multiple demos at the same time, assign different ports, especially for the two demos that both default to `8080`.

### Tests

Each demo is an independent Go module. Run the following command inside the `snake` directory you want to check:

```bash
go test ./...
```

### Related project

- [coolder](https://github.com/webcool-ai/coolder): the project providing the three AI coding apps, `coolder`, `coolder-java`, and `coolder-rust`, that wrote the demos in this repository.

### License

This repository is licensed under the [MIT License](./LICENSE).
