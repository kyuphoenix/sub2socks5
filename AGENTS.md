# AGENTS.md

## 适用范围与优先级

- 本文件适用于整个仓库。
- 如果子目录中存在更具体的 `AGENTS.md`，则子目录文件优先。
- 开始修改前先执行 `git status --short`，识别并保留用户已有的未提交改动；不要擅自还原、覆盖或提交与当前任务无关的文件。
- 除非用户明确要求，不要创建提交、推送远程、改写历史或执行破坏性 Git 操作。

## 项目概览

`sub2socks5` 是一个基于 Go 和 sing-box 的 SOCKS5 管理程序：

- 后端使用 Go 标准库 `net/http` 提供 API 和静态页面。
- 前端为无构建步骤的 HTML、CSS 和原生 JavaScript。
- `main.go` 使用 `go:embed` 将 `internal/public/` 打包进二进制文件。
- 程序负责订阅解析、节点管理、sing-box 配置生成、子进程生命周期、日志采集和内核下载。
- Go module 为 `sub2socks5-go`，当前 Go 版本以 `go.mod` 为准（目前为 Go 1.23）。
- 程序以当前工作目录为根目录，因此开发和运行命令应从仓库根目录执行。

## 目录说明

- `main.go`：程序入口及静态资源嵌入。
- `internal/app/app.go`：HTTP 路由、共享状态、订阅刷新、运行时进程和内核下载。
- `internal/app/subscription_parser.go`：订阅获取、URI/Base64/Clash YAML 解析及节点规范化。
- `internal/app/config_builder.go`：sing-box 配置、节点组和链式代理生成。
- `internal/app/*_test.go`：后端单元、并发、下载和运行时韧性测试。
- `internal/public/`：嵌入式 Web UI；直接编辑源文件，不要手工生成压缩版本。
- `.github/workflows/`：测试、跨平台构建和发布流程。
- `docs/diagnostics/`：故障诊断和根因记录。
- `docs/superpowers/plans/`：重要改动的实施计划。
- `internal/data/`、`internal/runtime/`、`internal/bin/`：运行期配置、生成文件和 sing-box 内核，不属于源码。
- `dist/`、`*.exe`、`*.db`：构建或运行产物，不应加入提交。

## 本地开发

### 前置条件

- Go：使用 `go.mod` 声明的版本或兼容的新版本。
- Node.js：仅用于对前端 JavaScript 执行语法检查，无 npm 依赖和打包步骤。
- sing-box：运行完整代理流程时需要；普通 Go 单元测试不应依赖真实 sing-box 或外部网络。

### 启动

```bash
go run .
```

默认 Web UI 地址为 `http://127.0.0.1:18080/` 或配置的监听地址。测试启动程序时必须确保退出后终止子进程，避免遗留端口和 sing-box 进程。

### 标准验证命令

后端改动至少执行：

```bash
gofmt -w main.go internal/app/*.go
go test ./...
go vet ./...
go build ./...
```

涉及共享状态、goroutine、HTTP、下载或子进程生命周期时，还必须执行：

```bash
go test -race ./...
```

涉及前端时执行：

```bash
node --check internal/public/app.js
node --check internal/public/nodes.js
node --check internal/public/nodes-edit.js
node --check internal/public/socks5.js
```

提交前执行：

```bash
git diff --check
git status --short
```

Windows 上的 LF/CRLF 转换警告不等同于 `git diff --check` 失败，但不要仅为消除警告而重写整个文件。

## 修改原则

- 优先做范围明确、可验证的修改，不进行与任务无关的大规模重构。
- 保持现有标准库优先的实现方式；引入新依赖前说明必要性，并同步更新 `go.mod` 和 `go.sum`。
- 所有 Go 文件必须通过 `gofmt`，命名和错误处理遵循惯用 Go 风格。
- 错误应带有操作上下文；不要静默忽略关键错误，尤其是文件、网络、`exec.Cmd` 管道和进程启动错误。
- 新增长耗时操作时接受或传递 `context.Context`，设置合理超时，并确保取消路径能释放 goroutine、timer、响应体、文件和子进程资源。
- 不要在日志、测试数据、文档或提交中写入真实订阅地址、代理凭据、节点密钥、访问令牌或服务器私密配置。
- 中文文档和源文件统一保存为 UTF-8。避免使用可能改变编码的 PowerShell 读取/管道/覆写链；写入时显式指定 UTF-8，并尽量只修改必要片段。

## 并发与长期运行约束

本项目曾出现长时间运行后管理程序卡住的问题。修改 `App`、订阅、HTTP handler、下载或 sing-box 生命周期时，以下规则是强制要求：

1. `App.mu` 只保护共享内存状态，临界区必须尽可能短。
2. 不得持有 `App.mu` 执行外部 HTTP 请求、等待响应体、等待进程、长时间文件 I/O、sleep 或其他不可控阻塞操作。
3. 不得持有 `App.mu` 调用 `http.ResponseWriter.Write`、`json.Encoder.Encode` 或向慢客户端发送响应。应在锁内复制/序列化状态，解锁后写响应。
4. 订阅刷新由 `subscriptionRefreshMu` 串行化；自动任务应避免堆积。不要引入不明确的嵌套锁顺序，也不要在持有两个锁时执行网络 I/O。
5. 共享 map、slice 或 `map[string]any` 在解锁后使用前必须复制或转换成不可变字节，避免数据竞争和并发修改。
6. 新 goroutine 必须有明确退出条件。所有 ticker、timer、channel 和 context 都要考虑关闭、取消及异常返回路径。
7. 缓冲区、日志行、请求体、下载体和队列必须有上限；禁止依赖无限字符串拼接或无限增长的内存集合。
8. 任何锁策略或后台任务改动都应添加回归测试，并运行 `go test -race ./...`。

详细历史根因见 `docs/diagnostics/long-running-hang-root-cause.md`。

## HTTP 与网络规则

- 保留 `newHTTPServer` 中明确的读 Header、读请求、写响应和空闲连接超时；新增服务端入口时也必须设置有限边界。
- API 请求体受大小限制。新增解析逻辑时，不要通过其他入口绕过限制；超限应返回 HTTP 413。
- 所有出站 HTTP client 必须有连接、TLS、响应头、总时长或空闲读取边界，不能使用无超时的默认行为处理长任务。
- HTTP 响应体必须关闭；对正文设置大小上限，并处理非 2xx 状态码。
- handler 发起的下载和刷新应绑定 `r.Context()`，客户端断开后应能够取消工作。
- 订阅源全部失败时保留最后一次成功状态，不得用空结果覆盖可用节点。
- 网络相关测试使用 `httptest.Server`、受控 reader 和本地同步机制；不要依赖真实公网服务。

## sing-box 运行时规则

- 区分人工启动、配置重启和异常退出后的自动重启，不能在自动重启成功时错误清零重试次数。
- 自动重启必须使用有上限的退避，并仅在稳定运行达到既定时间后重置退避状态。
- 启动进程时检查 `StdoutPipe`、`StderrPipe`、`Start` 等所有错误；失败路径不得遗留半初始化状态。
- 子进程日志采集必须限制单行和未换行缓冲大小，EOF 时提交剩余内容。
- 停止或替换进程时要处理人工停止标记，避免退出回调再次触发自动重启。
- 不要在测试中启动真实代理内核；将纯逻辑抽出后测试，或使用短生命周期的受控进程/reader。

## 配置、订阅与持久化

- 保持现有 JSON 字段和 API 兼容性。修改配置结构时提供默认值、规范化或迁移路径，并增加旧配置测试。
- `subscription_parser.go` 同时处理 URI、Base64 文本及 Clash/Mihomo YAML；新增协议或字段时至少添加一个有效样例和一个异常样例。
- 未知 Clash 类型当前存在兼容 fallback 行为。除非需求明确，不要把兼容数据直接变成硬错误。
- `config_builder.go` 输出必须符合 sing-box 结构，并维持内置 `proxy`、`auto`、`block` 出口及节点组/链式代理引用关系。
- 写入持久化文件时优先采用完整临时结果再替换的方式，避免失败时留下部分配置。
- 不要修改或提交本机生成的 `internal/data/`、`internal/runtime/`、`internal/bin/`、`cache.db` 和 `dist/` 内容。

## 前端规则

- 前端没有构建系统；修改 `internal/public/` 后重新编译 Go 二进制才能更新嵌入资源。
- 保持页面与后端 API 字段一致。API 字段变化时同步检查所有引用该字段的 HTML/JS 页面。
- 网络请求需要处理非成功状态、JSON 解析失败和用户可见错误，不要只在控制台记录。
- 避免引入需要额外 npm 构建流程的框架，除非用户明确要求并同步更新 CI、文档和发布流程。
- 修改 JavaScript 后至少对全部四个脚本执行 `node --check`。

## 测试要求

- 修复缺陷时先增加能够稳定复现问题的回归测试，再修改实现。
- 并发测试优先使用 channel、barrier 和有限 timeout 协调，不使用长时间 `Sleep` 猜测时序。
- 测试不得写入真实运行目录；使用 `t.TempDir()` 和测试专用配置。
- 测试必须可离线、可重复，不依赖系统代理、真实 GitHub Release、真实订阅源或固定外部端口。
- 改动解析器时运行解析器测试；改动配置生成时运行 builder 测试；改动锁、HTTP、下载、日志或重启逻辑时运行对应韧性测试及 race detector。
- 不要为了让测试通过而放宽生产超时、容量限制或错误检查。

## 故障诊断

长时间运行异常应先区分管理进程与 sing-box 子进程：

- Web API 也无响应：重点检查 Go goroutine、锁等待、网络 I/O 和 handler 堆积。
- Web API 正常但 SOCKS5 无流量：重点检查 sing-box 子进程、生成配置、目标节点和网络。
- Unix-like 系统可使用 `kill -QUIT <sub2socks5-pid>` 获取 goroutine dump；保留时间、PID、日志和复现操作。
- 不要在没有 goroutine dump、进程状态和日志证据时直接断言发生死锁。

## 完成任务前检查清单

1. 仅包含当前任务需要的文件，没有覆盖用户的既有改动。
2. 新行为有测试，错误和取消路径也得到覆盖。
3. Go 文件已格式化，前端文件已做语法检查。
4. 已执行与变更范围匹配的 `go test`、`go test -race`、`go vet` 和 `go build`。
5. `git diff --check` 通过，`git status --short` 中没有意外产物或敏感文件。
6. 最终回复准确列出修改文件、验证命令和未验证项；未实际执行的检查不得声称通过。
