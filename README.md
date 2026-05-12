# card_ssd — 十三张 Go 服务端

这是基于 **Gin + gorilla/websocket** 实现的十三张多人棋牌服务端，为本项目微信小游戏客户端提供大厅 HTTP 接口与对局 WebSocket 通信。

- HTTP 框架：[gin-gonic/gin](https://github.com/gin-gonic/gin)
- WebSocket 框架：[gorilla/websocket](https://github.com/gorilla/websocket)
- 通信分层：**主页与普通请求走 HTTP**，**对局相关实时通信走 WebSocket**
- 状态：用户档案 / 登录 token / 未结束房间 / 历史对局结果可选持久化到 **MySQL**；进程未配置 `MYSQL_PWD` 时整体降级为纯内存模式（不阻塞启动）
- 进行中的房间在所有真人离线后会保留 24 小时，由每小时巡检销毁
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

## 环境变量

| 变量名       | 必选 | 说明                                                                                       |
| ------------ | ---- | ------------------------------------------------------------------------------------------ |
| `PORT`       | 否   | HTTP 监听端口，默认 `80`                                                                   |
| `WX_APPID`   | 否   | 微信小游戏 AppID，配合 `WX_SECRET` 用于服务端解 `wx.login` 的 `code`；未配置时退化为读取请求头 `X-WX-OPENID`（云托管会自动注入） |
| `WX_SECRET`  | 否   | 微信小游戏 AppSecret，与 `WX_APPID` 配套                                                   |
| `MYSQL_PWD`  | 否   | MySQL 登录密码；**未配置时整体降级为内存模式**，所有 `Save*/Load*` 直接为 nil 兜底         |
| `MYSQL_HOST` | 否   | MySQL 地址，默认 `10.31.102.121:3306`                                                       |
| `MYSQL_USER` | 否   | MySQL 用户名，默认 `root`                                                                   |
| `MYSQL_DB`   | 否   | MySQL 库名，默认 `card_ssd`                                                                 |

## 微信登录链路（方案 B）

小游戏端 `js/main.js → initUser` 不再生成本地 `guest_xxxx` 作为 openid，而是异步调用 `wx.login` 取临时 `code`。`js/scenes/lobby_scene.js → _httpLogin` 把 `{ code, nickname, avatarUrl }` 提交到 `POST /api/login`：

1. 服务端优先用 `code` 调用 `https://api.weixin.qq.com/sns/jscode2session` 解出真实 `openid`（凭 `WX_APPID/WX_SECRET`）
2. `code` 失败时退化为读取请求头 `X-WX-OPENID`（云托管侧自动注入）
3. 兼容 body.openid（仅本地调试 / 老客户端）
4. 三步均失败 → 返回 `400 登录失败`

服务端会把 `openid → nickname/avatarUrl` UPSERT 到 `users` 表，并在客户端未带昵称头像时从该表回填，避免「玩家xxxx」。客户端拿到响应后会把 `openid` 写回 `wx.setStorageSync('openid', ...)` 与 `databus.user.openid`，后续 WS `LOGIN` 帧、`/api/lobby/active-room` 都使用真实 openid。

## MySQL 持久化

服务启动时 `internal/storage` 包会按需建立 MySQL 连接池并自动执行内置 DDL（`CREATE TABLE IF NOT EXISTS`）。表结构如下：

| 表名            | 主要字段 / 用途                                                                                                                  |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `users`         | `openid PK / nickname / avatar_url / created_at / updated_at`，存放微信授权得到的昵称头像                                          |
| `auth_tokens`   | `token PK / openid / nickname / avatar_url / expires_at`，HTTP 登录颁发的 7 天 token；启动时清理过期记录                            |
| `rooms`         | `room_id PK / host_openid / phase / current_round / total_rounds / max_players / with_ma / last_active_at / all_offline_since / destroyed`，未结束房间快照 |
| `room_players`  | `(room_id, openid) PK / seat / nickname / avatar_url / score / is_bot / offline / hand(JSON) / lanes(JSON) / submitted / round_confirmed / vote_dissolve` |
| `match_results` | `id PK / room_id / round / with_ma / total_rounds / payload(JSON) / created_at`，每局结算结果，便于后续做战绩 / 历史回顾            |

关键设计：

- **降级策略**：未配置 `MYSQL_PWD` / 连接失败 / 迁移失败时，`storage.DB() == nil`，所有 `Save*/Load*` 直接 `return`，业务逻辑保持纯内存运行。
- **节流写盘**：`internal/room/persister.go` 维护房间维度 `dirty` 标记，后台 1 秒批量落库 `rooms` + `room_players`；进程退出 / 房间销毁等关键节点立即同步刷盘。
- **启动恢复**：`room.LoadFromStorage()` 会把 `destroyed=0` 的房间及玩家全量还原到内存，所有玩家初始为 `Offline=true` 等待真人重连，由既有 24h 巡检负责销毁过期房间。
- **错误处理**：写库失败一律 `WARN` 不向上抛 panic，不阻塞当前请求。

客户端默认走微信云托管通道（`wx.cloud.callContainer` / `wx.cloud.connectContainer`），本地调试可在 [js/main.js](../js/main.js) 中调整 `GameGlobal.HTTP_BASE` / `GameGlobal.SOCKET_URL` 为本地地址，并在浏览器/无 `wx.cloud` 环境下自动降级使用。

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
    │   └── settle.go             // 比牌结算（特殊加分、打枪、本垒打、马牌倍率；本垒打支持 2-6 人，2 人场打枪对方即同时构成本垒打 ×2）
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
| `POST` | `/api/login`               | 登录，body `{ openid, nickname, avatarUrl }`（`nickname/avatarUrl` 由客户端通过 `wx.createUserInfoButton` 由用户主动授权后透传，会覆盖 `Session.Nickname/AvatarUrl`），返回 `{ token, openid, nickname, activeRoom }`；`activeRoom` 为该 openid 当前未结束房间的摘要或 `null` |
| `GET`  | `/api/lobby/active-room`   | 仅查询 `activeRoom`，query `?openid=xxx`，返回 `{ activeRoom }`；**只读**，不修改任何在线状态 |
| `POST` | `/api/room/create`         | 创建房间，需 token，body `{ withMa, totalRounds }`（房间人数上限服务端固定为 6，开局门槛为就绪人数 ≥ 2），返回 `{ roomId }` |
| `GET`  | `/api/room/:id`            | 查询房间概要，不存在返回 404                                            |

> token 通过 query `?token=` 或 header `X-Token` / `Authorization: Bearer <token>` 携带均可。
> HTTP 创建仅生成房间 ID，玩家实际加入仍需走 WebSocket `JOIN_ROOM`。
> 客户端大厅启动后仅调用 `POST /api/login` 一次，不建立 WebSocket；只有在点击“创建 / 加入 / 重新进入”时才升级到 `/ws`。

## WebSocket 协议（`/ws`）

JSON 信封：`{ type, data, reqId, code, msg }`。

| 类型                                              | 方向   | 说明                              |
| ------------------------------------------------- | ------ | --------------------------------- |
| `LOGIN`                                           | C→S    | 登录（携带 `openid/nickname/avatarUrl`，会覆盖 `Session.Nickname/AvatarUrl`，进房间 / `UpsertPlayer` 时同步到 `Player`，最终随 `ROOM_STATE` 广播） |
| `CREATE_ROOM` / `JOIN_ROOM` / `LEAVE_ROOM`        | C→S    | 房间操作                          |
| `READY` / `UNREADY`                               | C→S    | 准备状态切换                      |
| `SUBMIT_LANES`                                    | C→S    | 提交三道开牌                      |
| `ROUND_CONFIRM`                                   | C→S    | 确认本局结算                      |
| `ROOM_ADD_BOT` / `ROOM_KICK_BOT`                  | C→S    | 房主添加/踢出电脑玩家              |
| `ROOM_KICK_PLAYER`                                | C→S    | 房主踢出真人玩家（waiting / match_end 阶段） |
| `ROOM_KICKED`                                     | S→C    | 被踢玩家单播通知                  |
| `VOTE_DISSOLVE` / `VOTE_DISSOLVE_CANCEL`          | C→S    | 发起/同意 / 撤销投票解散         |
| `VOTE_DISSOLVE_TIMEOUT`                           | S→C    | 投票解散 60 秒超时广播           |
| `LOGIN_OK` / `CREATE_ROOM_OK` / `JOIN_ROOM_OK`    | S→C    | 应答                              |
| `LEAVE_ROOM_OK` / `SUBMIT_LANES_OK`               | S→C    | 应答                              |
| `ROOM_ADD_BOT_OK` / `ROOM_KICK_BOT_OK`            | S→C    | 电脑玩家操作应答                  |
| `ROOM_KICK_PLAYER_OK`                             | S→C    | 踢出真人玩家应答                  |
| `ROOM_STATE`                                      | S→C    | 房间状态广播                      |
| `DEAL_CARDS`                                      | S→C    | 私发手牌                          |
| `SETTLE_RESULT`                                   | S→C    | 比牌结算广播                      |
| `RECONNECT_SNAPSHOT`                              | S→C    | 断线重连快照，恢复手牌、三道、结算与当前阶段 |
| `MATCH_END`                                       | S→C    | 整场结束总排行                    |
| `ERROR`                                           | S→C    | 错误（含 code/msg）               |

升级 URL 可选携带 `?token=...` 用于预先绑定身份（HTTP 登录后获取）；亦可在升级后通过 `LOGIN` 消息绑定。

## 数据生命周期

- 用户档案 / 登录 token / 未结束房间 / 历史结算 在配置 `MYSQL_PWD` 时持久化到 MySQL；未配置时降级为纯内存（重启即清空）
- 房间内最后一名玩家**主动离开**时，房间立即从内存销毁并写 `rooms.destroyed=1`
- **房间 24 小时保活**：房间处于任何阶段（`waiting/playing/comparing/match_end`）且所有真人玩家都离线时，服务端会登记 `AllOfflineSince`，任一真人重连即清零；`StartIdleSweeper` 每小时巡检一次，销毁“超过 24 小时仍未有真人上线”的房间
- 服务端进程重启：通过 `room.LoadFromStorage()` 把 `destroyed=0` 的房间整体还原到内存，全员视为离线，等待玩家重连后由原有重连流程恢复手牌 / 三道 / 阶段
- 玩家断线 30 秒未重连的阶段化兜底：
  - 准备阶段（`waiting`）：仅标记 `Offline=true`，**不启动 30 秒踢出计时器**，仅由 24h 巡检接管；玩家可从大厅「重新进入」回房间继续等待开局
  - 对局阶段（`playing`）：自动以头 3/中 5/尾 5 散牌参与结算
  - 比牌阶段（`comparing`）：保留座位等待重连或整场结束（积分已固定）
  - 整场结束（`match_end`）：移除该玩家，避免永久占座
- 30 秒兜底执行后若房间内已无在线真人，不再立即销毁，而是交由 24h 巡检处理；bot 定时器与投票定时器在房间销毁时统一取消
- 投票解散：所有在线真人均 `voteDissolve=true` 时立即跳转 `match_end` 并按累计积分提前结算，首次投票触发 60 秒倒计时，超时作废
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

// 房主踢出真人玩家（仅 waiting / match_end 阶段允许）
{ "type": "ROOM_KICK_PLAYER", "data": { "openid": "o_xxx" }, "reqId": 3 }
// 服务端会先向被踢玩家单播：
{ "type": "ROOM_KICKED", "data": { "roomId": "1234", "reason": "host_kick" } }
// 随后向房主回应：
{ "type": "ROOM_KICK_PLAYER_OK", "data": { "openid": "o_xxx" }, "reqId": 3 }
```

### 服务端行为

- **加入**：返回后 1 秒内自动设为已准备，并广播 `ROOM_STATE`
- **发牌**：进入 `playing` 后 1 秒内调用 `game.AutoArrange()` 理牌，随后提交开牌（复用 `SUBMIT_LANES` 路径）
- **结算/总结**：进入 `comparing`/`match_end` 后 1 秒内自动确认，避免房间销毁流程被阻塞
- **销毁清理**：房间销毁时统一停止 `botTimers`，避免协程泄漏
- **限制**：仅房主、仅 `waiting` 阶段、仅在未达 `rule.maxPlayers` 时才允许添加

## 房主踢出真人玩家

除了踢出电脑之外，房主还可在 `waiting` / `match_end` 阶段踢出任意非自身的真人玩家，复用同一个座位点击交互。

### 服务端行为

- **鉴权**：非房主返回 `ErrNotHost(1009)`；阶段不是 `waiting` 或 `match_end` 返回 `ErrRoomNotWaiting(1010)`；目标等于调用者 openid 返回 `ErrBadRequest(1008)`；目标不在房间返回 `ErrBadRequest`。
- **踢出顺序**：先向被踢玩家单播 `ROOM_KICKED`（携带 `roomId` / `reason="host_kick"`）并清空其 `Session.RoomID`，随后调用 `CancelOfflineTimer` 释放可能挂起的离线计时器，再从 `r.Players` 中移除。
- **后续处理**：房间仍有真人时广播新的 `ROOM_STATE`；如果踢出后无任何真人则走与 `LeaveRoom` 一致的房间销毁流程（含 bot 定时器清理与持久层销毁标记）。
- **日志审计**：`logger.Info("房间 X 房主 Y 踢出真人玩家 Z(昵称)")`。
- **与投票解散兼容**：被踢玩家随 `RemovePlayer` 从投票集合中自动退出，不需额外处理。

### AI 理牌策略

AI 位于 [internal/game/ai_bot.go](./internal/game/ai_bot.go)：枚举 尾道 13C5 × 中道 8C5，选择“尾道牌型最强 + 中道 ≤ 尾道 + 头道 ≤ 中道”的最优三道。若枚举未能产出合法三道，则走“点数从大到小填尾/中/头”的兑底策略。兑底会输出 WARN 日志便于排查。

## 主页 HTTP 化 / 对局 WS / 断线视觉

- **大厅仅 HTTP**：客户端进入大厅后调用 `POST /api/login` 一次性拿到身份与 `activeRoom`；不再在启动期间建立 WebSocket。`activeRoom` 返回须严格只读，不影响 `Player.Offline` 状态。
- **云托管通道**：前端默认使用微信云托管 `wx.cloud.callContainer` / `wx.cloud.connectContainer` 访问本服务，服务名与环境 ID 通过 `GameGlobal.CLOUD_SERVICE` / `GameGlobal.CLOUD_ENV` 配置；服务端对云托管与直连请求一视同仁，`/api/...` 与 `/ws` 路由保持不变。
- **对局才升级 WS**：创建 / 加入 / 重新进入三个动作会先连接 `/ws`，等待 `LOGIN_OK` 后再发送 `CREATE_ROOM / JOIN_ROOM`。对局结束后客户端会主动关闭 Socket。
- **顶号保护**：同 openid 新连接登录时，服务端会关闭旧 underlying socket，但保留该玩家在房间内的位置以供重连；旧 session 关闭触发的 `HandleDisconnect` 会判定 `byOpenid` 已被新 session 接管后跳过，避免误标为“离线”。
- **离线/重连广播**：`Player.Offline=true` 后立即 `BroadcastState`；30 秒兑底后仍保持广播直到玩家被移除或房间销毁；`JOIN_ROOM` 重连重置 `Offline=false / OfflineSince=0` 后也广播一次。
- **重连快照与结算兜底**：`JOIN_ROOM` 命中重连分支后，服务端单播 `RECONNECT_SNAPSHOT`，携带 `phase / hand / lanes / submitted / lastSettle / currentRound / totalRounds`；若重连时发现房间仍在 `playing` 但全员已开牌，会先触发结算，避免客户端卡在“已开牌，等待其他玩家”。
- **头像离线视觉**：客户端在 `player.offline === true` 且非本机人、非 Bot 时，于头像上叠加半透明圆形蒙层与“OFF”字样；玩家恢复后蒙层自动移除。
