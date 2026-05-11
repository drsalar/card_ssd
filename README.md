# card_ssd — 十三张 Go 服务端

这是基于 **Gin + gorilla/websocket** 实现的十三张多人棋牌服务端，为本项目微信小游戏客户端提供大厅 HTTP 接口与对局 WebSocket 通信。

- HTTP 框架：[gin-gonic/gin](https://github.com/gin-gonic/gin)
- WebSocket 框架：[gorilla/websocket](https://github.com/gorilla/websocket)
- 通信分层：**主页与普通请求走 HTTP**，**对局相关实时通信走 WebSocket**
- 状态：所有房间/对局信息保留在服务端进程内存中，所有玩家退出后立即销毁
- 语言：Go 1.24+

## 启动方式

```powershell
cd card_ssd
go mod tidy
go run .
```

默认监听 `:80`，可通过环境变量 `PORT` 覆盖：

```powershell
$env:PORT="9090"; go run .
```

服务启动后：

- WebSocket 地址：`ws://127.0.0.1/ws`
- HTTP 健康检查：`http://127.0.0.1/api/health`

客户端只需将 [js/main.js](../js/main.js) 中 `SOCKET_URL` 指向 `ws://127.0.0.1/ws` 即可。

## 目录结构

```
card_ssd
├── main.go                       // 入口：构建 Gin 引擎、监听端口、优雅关闭
├── go.mod
├── README.md
└── internal
    ├── protocol/protocol.go      // 协议常量（消息类型、错误码）
    ├── logger/logger.go          // 日志模块（INFO/WARN/ERROR）
    ├── game
    │   ├── card.go               // 卡牌、牌堆、洗牌、加色规则
    │   ├── evaluator.go          // 牌型识别与比较（10 种牌型）
    │   ├── validator.go          // 三道大小校验
    │   └── settle.go             // 比牌结算（特殊加分、打枪、本垒打、马牌倍率）
    ├── session
    │   ├── session.go            // 单个 WS 会话：发送/接收/Ping 保活
    │   └── manager.go            // connID/openid/token 三索引、顶号、批量关闭
    ├── room
    │   ├── room.go               // 房间数据结构、广播
    │   └── manager.go            // 创建/加入/离开/销毁/断线判定（30 秒兜底）
    ├── handler
    │   ├── lobby.go              // 登录、创建/加入/离开、准备
    │   └── game.go               // 发牌、提交三道、结算、确认本局结算
    └── server
        ├── http.go               // Gin 路由：HTTP API + CORS
        └── ws.go                 // /ws 升级 + 消息分发 + 优雅关闭
```

## HTTP 路由

| Method | Path                       | 说明                                                                   |
| ------ | -------------------------- | ---------------------------------------------------------------------- |
| `GET`  | `/api/health`              | 健康检查，返回 `{ ok, time, rooms, conns }`                            |
| `POST` | `/api/login`               | 登录，body `{ openid, nickname, avatarUrl }`，返回 `{ token, openid, nickname, activeRoom }`；`activeRoom` 为该 openid 当前未结束房间的摘要或 `null` |
| `GET`  | `/api/lobby/active-room`   | 仅查询 `activeRoom`，query `?openid=xxx`，返回 `{ activeRoom }`；**只读**，不修改任何在线状态 |
| `POST` | `/api/room/create`         | 创建房间，需 token，body `{ withMa, totalRounds, maxPlayers }`，返回 `{ roomId }` |
| `GET`  | `/api/room/:id`            | 查询房间概要，不存在返回 404                                            |

> token 通过 query `?token=` 或 header `X-Token` / `Authorization: Bearer <token>` 携带均可。
> HTTP 创建仅生成房间 ID，玩家实际加入仍需走 WebSocket `JOIN_ROOM`。
> 客户端大厅启动后仅调用 `POST /api/login` 一次，不建立 WebSocket；只有在点击“创建 / 加入 / 重新进入”时才升级到 `/ws`。

## WebSocket 协议（`/ws`）

JSON 信封：`{ type, data, reqId, code, msg }`。

| 类型                                              | 方向   | 说明                              |
| ------------------------------------------------- | ------ | --------------------------------- |
| `LOGIN`                                           | C→S    | 登录（携带 openid/昵称/头像）     |
| `CREATE_ROOM` / `JOIN_ROOM` / `LEAVE_ROOM`        | C→S    | 房间操作                          |
| `READY` / `UNREADY`                               | C→S    | 准备状态切换                      |
| `SUBMIT_LANES`                                    | C→S    | 提交三道开牌                      |
| `ROUND_CONFIRM`                                   | C→S    | 确认本局结算                      |
| `ROOM_ADD_BOT` / `ROOM_KICK_BOT`                  | C→S    | 房主添加/踢出电脑玩家              |
| `LOGIN_OK` / `CREATE_ROOM_OK` / `JOIN_ROOM_OK`    | S→C    | 应答                              |
| `LEAVE_ROOM_OK` / `SUBMIT_LANES_OK`               | S→C    | 应答                              |
| `ROOM_ADD_BOT_OK` / `ROOM_KICK_BOT_OK`            | S→C    | 电脑玩家操作应答                  |
| `ROOM_STATE`                                      | S→C    | 房间状态广播                      |
| `DEAL_CARDS`                                      | S→C    | 私发手牌                          |
| `SETTLE_RESULT`                                   | S→C    | 比牌结算广播                      |
| `RECONNECT_SNAPSHOT`                              | S→C    | 断线重连快照，恢复手牌、三道、结算与当前阶段 |
| `MATCH_END`                                       | S→C    | 整场结束总排行                    |
| `ERROR`                                           | S→C    | 错误（含 code/msg）               |

升级 URL 可选携带 `?token=...` 用于预先绑定身份（HTTP 登录后获取）；亦可在升级后通过 `LOGIN` 消息绑定。

## 数据生命周期

- 所有房间与对局信息仅保存在服务端进程内存中
- 房间内最后一名玩家离开时，房间立即从内存销毁
- 玩家断线 30 秒未重连的阶段化兜底：
  - 准备阶段（`waiting`）：连接断开瞬间即移除
  - 对局阶段（`playing`）：自动以头 3/中 5/尾 5 散牌参与结算
  - 比牌阶段（`comparing`）：保留座位等待重连或整场结束（积分已固定）
  - 整场结束（`match_end`）：移除该玩家，避免永久占座
- 兜底执行后若房间内已无在线真人，立即销毁房间，避免与 bot 持续空转
- 房主退出：房主权限自动转交给最早加入的剩余玩家
- 同一 openid 重复登录：旧连接自动断开，身份指向新连接

## 同 openid 多连接

处理要点：

- **并发安全**：房间级 `sync.Mutex` + Session 写串行化
- **优雅退出**：监听 `SIGINT/SIGTERM`，关闭所有 WS 连接
- **跨域**：HTTP 启用 CORS（任意 Origin），WebSocket Upgrade 不检查 Origin

## 电脑玩家（AI Bot）

房主可在“准备阶段”为房间添加、踢出电脑玩家，电脑玩家由服务端统一驱动，与真人玩家共用同一套协议。

### 协议示例

```jsonc
// 房主添加电脑玩家
{ "type": "ROOM_ADD_BOT", "data": {}, "reqId": 1 }
// 服务端应答
{ "type": "ROOM_ADD_BOT_OK", "data": { "openid": "bot_1234_1", "nickname": "电脑1" }, "reqId": 1 }

// 房主踢出电脑玩家
{ "type": "ROOM_KICK_BOT", "data": { "openid": "bot_1234_1" }, "reqId": 2 }
```

### 服务端行为

- **加入**：返回后 1 秒内自动设为已准备，并广播 `ROOM_STATE`
- **发牌**：进入 `playing` 后 1 秒内调用 `game.AutoArrange()` 理牌，随后提交开牌（复用 `SUBMIT_LANES` 路径）
- **结算/总结**：进入 `comparing`/`match_end` 后 1 秒内自动确认，避免房间销毁流程被阻塞
- **销毁清理**：房间销毁时统一停止 `botTimers`，避免协程泄漏
- **限制**：仅房主、仅 `waiting` 阶段、仅在未达 `rule.maxPlayers` 时才允许添加

### AI 理牌策略

AI 位于 [internal/game/ai_bot.go](./internal/game/ai_bot.go)：枚举 尾道 13C5 × 中道 8C5，选择“尾道牌型最强 + 中道 ≤ 尾道 + 头道 ≤ 中道”的最优三道。若枚举未能产出合法三道，则走“点数从大到小填尾/中/头”的兑底策略。兑底会输出 WARN 日志便于排查。

## 主页 HTTP 化 / 对局 WS / 断线视觉

- **大厅仅 HTTP**：客户端进入大厅后调用 `POST /api/login` 一次性拿到身份与 `activeRoom`；不再在启动期间建立 WebSocket。`activeRoom` 返回须严格只读，不影响 `Player.Offline` 状态。
- **对局才升级 WS**：创建 / 加入 / 重新进入三个动作会先连接 `/ws`，等待 `LOGIN_OK` 后再发送 `CREATE_ROOM / JOIN_ROOM`。对局结束后客户端会主动关闭 Socket。
- **顶号保护**：同 openid 新连接登录时，服务端会关闭旧 underlying socket，但保留该玩家在房间内的位置以供重连；旧 session 关闭触发的 `HandleDisconnect` 会判定 `byOpenid` 已被新 session 接管后跳过，避免误标为“离线”。
- **离线/重连广播**：`Player.Offline=true` 后立即 `BroadcastState`；30 秒兑底后仍保持广播直到玩家被移除或房间销毁；`JOIN_ROOM` 重连重置 `Offline=false / OfflineSince=0` 后也广播一次。
- **重连快照与结算兜底**：`JOIN_ROOM` 命中重连分支后，服务端单播 `RECONNECT_SNAPSHOT`，携带 `phase / hand / lanes / submitted / lastSettle / currentRound / totalRounds`；若重连时发现房间仍在 `playing` 但全员已开牌，会先触发结算，避免客户端卡在“已开牌，等待其他玩家”。
- **头像离线视觉**：客户端在 `player.offline === true` 且非本机人、非 Bot 时，于头像上叠加半透明圆形蒙层与“OFF”字样；玩家恢复后蒙层自动移除。
