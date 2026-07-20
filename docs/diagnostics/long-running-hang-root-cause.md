# 长时间运行后卡住：根因分析与整改清单

日期：2026-07-19

## 现象

程序启动初期正常，运行一段时间后 Web API、运行状态查询或代理管理操作可能长时间没有响应，看起来像整个程序卡住。

## 已确认根因

### P0-1：订阅网络请求期间持有全局写锁

位置：`internal/app/app.go` 的自动订阅调度和手动订阅刷新流程。

原行为：先获取 `App.mu.Lock()`，再请求远程订阅。单个订阅最多等待 20 秒，多个订阅串行时等待时间会累加。写锁存在期间，配置读取、日志查询、启动/停止内核等所有依赖该锁的 API 都会等待。

复现证据：`TestSubscriptionRefreshDoesNotHoldAppLockDuringNetworkIO` 在旧实现上稳定失败。

整改：使用独立的订阅任务锁避免重复刷新；全局锁只用于复制配置和提交结果，所有远程 I/O 均在全局锁之外执行。

状态：[x] 已修正（订阅专项回归测试通过）

### P0-2：向慢客户端写 HTTP 响应时仍持有全局读锁

位置：配置、节点、运行日志等 GET API。

原行为：通过 `defer RUnlock()` 将锁持有到 `json.Encoder.Encode` 完成。如果客户端网络慢、连接半断开或响应写入阻塞，写锁会排队；Go 的 `RWMutex` 在写锁排队后会阻止新的读锁，最终表现为后续 API 全部卡住。

复现证据：`TestHandleRuntimeLogsReleasesLockBeforeWritingResponse` 在旧实现上稳定失败。

整改：在锁内生成不可变快照或完成 JSON 序列化，释放锁后再向网络写响应；审计全部共享 map/slice 响应，消除解锁后并发编码共享对象的数据竞争。

状态：[x] 已修正（慢响应写入回归测试及 `go test -race ./internal/app` 通过）

### P0-3：订阅源全部失败时会用空节点覆盖最后一次可用状态

位置：`fetchSubscription` 与手动/自动刷新提交逻辑。

原行为：HTTP 失败只写入 warnings，但刷新仍被当成成功，并把 `nodes=[]` 保存。后续保存配置或重启 sing-box 时可能生成没有有效代理节点的配置，造成代理不可用。

复现证据：`TestSubscriptionRefreshPreservesLastGoodStateWhenAllSourcesFail` 在旧实现上稳定失败，旧接口返回 200 且节点被清空。

整改：统计尝试源和成功源；全部失败时返回上游错误、记录日志并保留最后一次成功状态。

状态：[x] 已修正（订阅专项回归测试通过）

## 高风险放大因素

### P1-1：HTTP 服务没有超时和请求体上限

原行为：使用裸 `http.ListenAndServe`，没有 `ReadHeaderTimeout`、`ReadTimeout`、`WriteTimeout`、`IdleTimeout`；API JSON 请求体直接 `io.ReadAll`。

风险：慢连接和异常客户端可长期占用连接/goroutine；大请求体可造成内存压力。

状态：[x] 已修正（显式 HTTP Server 超时、64 KiB 请求头上限、2 MiB API 请求体上限，超限返回 413）

### P1-2：下载和订阅响应缺少完整边界

原行为：内核下载客户端 `Timeout: 0`，连接或响应流停滞时可无限等待；订阅正文无大小上限；下载进度每 64 KiB 获取一次全局锁。

整改：增加总超时、正文/文件大小上限，并降低进度状态更新频率。

状态：[x] 已修正（订阅 16 MiB；内核包 512 MiB；下载总超时 30 分钟、空闲超时 2 分钟，进度更新已节流）

### P1-3：子进程日志的“未换行内容”可无限增长

原行为：`captureLogs` 的 `acc` 在子进程持续输出但不换行时无限累积，并反复进行字符串拼接。

整改：限制单条日志和未完成行的最大长度，EOF 时安全提交剩余内容。

状态：[x] 已修正（单条日志限制 16 KiB，截断标记，EOF 提交剩余内容）

### P1-4：崩溃重启计数每次启动都会清零

原行为：自动重启调用 `startRuntimeLocked` 后立刻把 `autoRestartAttempts` 设为 0，导致连续崩溃永远按第一次重试，形成固定 2 秒的崩溃循环。

整改：只有人工启动/配置重启才清零；自动重启保留计数并执行递增退避，稳定运行一段时间后再重置。

状态：[x] 已修正（自动重启保留计数，2/4/8/16/30 秒指数退避，稳定运行 5 分钟后重置）

## 按顺序执行的整改步骤

1. [x] 完成订阅刷新解锁与失败保底；让两条订阅回归测试转绿。
2. [x] 修正所有锁内 HTTP 写响应及共享对象快照；让慢写入回归测试转绿。
3. [x] 增加 HTTP 服务超时、请求体上限及对应测试。
4. [x] 增加订阅/下载大小与超时边界，节流下载进度更新。
5. [x] 限制运行日志行大小，修正自动重启退避计数，并增加测试。
6. [x] 执行 `gofmt`、`go test ./...`、`go test -race ./...`、`go vet ./...`、`go build ./...` 和前端 JavaScript 语法检查。
7. [x] 记录最终改动、仍需服务器现场观察的指标及抓取 Go goroutine 堆栈的方法。

## 实际验证结果

验证日期：2026-07-19

以下命令均已实际执行：

- `gofmt -w main.go internal/app/*.go`：完成。
- `go test ./...`：通过，`internal/app` 包全部测试通过。
- `go test -race ./...`：通过，未报告数据竞争。
- `go vet ./...`：通过。
- `go build ./...`：通过。
- `node --check internal/public/app.js`：通过。
- `node --check internal/public/nodes.js`：通过。
- `node --check internal/public/nodes-edit.js`：通过。
- `node --check internal/public/socks5.js`：通过。
- `git diff --check`：通过（只有 Git 的 LF/CRLF 提示，无空白错误）。

新增回归测试覆盖：慢 HTTP 响应写入时全局锁释放、订阅 I/O 期间全局锁释放、订阅全部失败时保留最后可用状态、HTTP 请求体超限、下载超时/大小/进度节流、日志截断及自动重启退避。

## 服务器升级后观察项

1. 观察 Web API 的响应时间，尤其是 `/api/runtime/logs`、`/api/config` 和 `/api/subscription/refresh`。
2. 观察管理进程的 RSS/内存是否持续增长，并记录异常发生前后的内存和 CPU。
3. 检查运行日志中的 `auto-restart` 次数和退避时间，确认连续崩溃时不再固定 2 秒循环。
4. 订阅失败时确认旧节点仍然可用，并保留 warnings 和失败时间。
5. 内核下载时观察 `/api/kernel/download` 状态；如超时或停滞，应进入 error 状态，而不是无限期 active。

## 如果仍然卡住，现场抓取方法

Linux 上向 Go 管理进程发送 `SIGQUIT`，可将当前所有 goroutine 堆栈输出到进程标准错误/服务日志：

```bash
kill -QUIT <sub2socks5-pid>
```

如果使用 systemd，同时保存对应时段的 `journalctl -u <service-name>` 输出。现场需要先区分：

1. **Web API 也无响应**：优先排查 Go 管理进程，保存 goroutine dump、CPU、RSS、打开文件数和当时正在调用的 API。
2. **Web API 正常，但 SOCKS5 无流量**：优先排查 sing-box 子进程、节点连通性及生成的 `internal/runtime/sing-box.json`，并保存 sing-box 自身日志。

## 不直接下结论的部分

目前没有服务器卡住时的 goroutine dump，因此不能断言 sing-box 内核自身一定发生死锁。本次先修复已经可以从代码和回归测试确认的管理进程全局阻塞问题。若修复后仍出现代理流量停滞但 Web API 正常，需要在现场进一步区分“管理进程卡住”和“sing-box 子进程存活但不可用”。
