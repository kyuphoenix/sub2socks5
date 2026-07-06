# Background（开发背景与实现说明）

本文档记录 `sub2socks5` 当前实现、关键设计取舍、兼容策略与验证方法，便于后续继续开发。

---

## 1. 技术栈

### 后端

- 语言：`Go 1.23+`
- 主要标准库：
  - `net/http`：Web UI 与 API 服务。
  - `os`、`filepath`：目录、配置与运行时文件管理。
  - `encoding/json`：配置与状态序列化。
  - `os/exec`：sing-box 进程管理。
  - `net`：端口探测与监听冲突规避。
- 第三方库：
  - `gopkg.in/yaml.v3`：Clash/Mihomo YAML 解析。

### 前端

- 原生 `HTML + CSS + JavaScript`。
- 通过 `fetch` 调用后端 API。
- 表单视图与 JSON 视图可切换。
- 运行状态页包含状态与日志选项卡。

### 代理内核

- `sing-box` 负责出站协议、DNS、路由、入站 SOCKS5 监听与原生分组策略。

---

## 2. 目录与持久化

源码运行时以项目目录为根目录，打包后二进制以自身所在目录为根目录。

- `internal/data/app-config.json`：主配置。
- `internal/data/subscription-state.json`：订阅原文、解析节点、警告信息。
- `internal/data/release-list.json`：sing-box Release 列表缓存。
- `internal/runtime/sing-box.json`：生成的 sing-box 配置。
- `internal/bin/`：下载的 sing-box 内核。

启动时会自动创建缺失目录与默认配置，避免首次运行因文件不存在失败。

---

## 3. 模块划分

- `internal/app/app.go`
  - API 路由、配置读写、订阅刷新、运行时启动/停止、内核下载、日志收集。
- `internal/app/subscription_parser.go`
  - URI 订阅、Base64 订阅、Clash/Mihomo YAML 与兼容兜底解析。
- `internal/app/config_builder.go`
  - 合并订阅节点和手动节点，生成 sing-box `inbounds`、`outbounds`、`route`、`dns`、`experimental` 配置。
- `internal/public/*.js`
  - Web UI 状态管理、表单编辑、节点管理、SOCKS5 服务配置与一键配置流程。

---

## 4. 订阅解析策略

### URI 订阅

支持：

- `vmess://`
- `vless://`
- `trojan://`
- `ss://`
- `socks://` / `socks5://`
- `hysteria2://` / `hy2://`
- `tuic://`

解析时会处理常见 TLS、Reality、transport、grpc、ws、tcp 等参数，并尽量转换为 sing-box 出站结构。

### Clash/Mihomo YAML

当普通 URI 解析为空时，会尝试按 YAML 解析并读取 `proxies`。

当前覆盖类型：

- `ss`
- `trojan`
- `vmess`
- `vless`
- `socks` / `socks5`
- `http`
- `hysteria2` / `hy2`
- `tuic`
- `anytls`
- `ssr`（兼容映射）
- `snell`
- `wireguard`

未知类型会进入兜底策略：尝试按 `http`、`shadowsocks`、`socks` 形态映射。兜底节点会附带：

- `tag` 前缀：`[fallback]`
- `compat_fallback: true`
- `compat_origin_type: <原始类型>`

---

## 5. 节点组策略

### urltest

生成 sing-box 原生 `urltest` 出站，根据测试地址、间隔和容差选择较优成员。一键配置 SOCKS5 服务时，会为全部可用节点创建一个 `urltest` 节点组作为首个服务出口。

### fallback

生成 sing-box 原生 `selector` 出站，默认选择成员列表第一个节点。当前 UI 保留状态展示入口，后续可以继续增强为更完整的健康检查切换。

### 配置迁移

配置加载和保存时会执行规范化：

- 空策略名会补为 `urltest`。
- 非原生历史策略名会改写为 `urltest`。
- 历史运行态缓存会从主配置中移除。

---

## 6. DNS 与运行时

- 默认 DoH：`https://cloudflare-dns.com/dns-query`。
- 默认 bootstrap DNS：`1.1.1.1`。
- DoH 查询默认通过 `proxy` 绕行，目标是降低本机明文 DNS 泄漏。
- 运行时配置保存到 `internal/runtime/sing-box.json`。
- 如果 sing-box 正在运行，保存配置后会重启运行时以应用配置。

---

## 7. 内核管理

Web UI 支持：

- 检测当前系统架构。
- 从缓存读取 Release 列表。
- 手动检查版本更新。
- 手动选择系统架构和版本。
- 下载并替换计划版本内核。

Release 资产选择会避开 Windows legacy 版本，并按系统与架构匹配对应压缩包。

---

## 8. 测试覆盖

当前测试重点：

- `subscription_parser_test.go`
  - Clash YAML 基础解析。
  - 未知类型 fallback 标记。
- `config_builder_test.go`
  - `urltest` 节点组生成 sing-box 原生 `urltest`。
  - 历史策略名迁移为 `urltest`。
  - SOCKS5 入站端口保持用户配置，不做额外重写。
  - `collectOutbounds` 输出节点、节点组、链式代理和内置出口。

推荐验证命令：

```bash
go test ./...
go build ./...
node --check internal/public/app.js
node --check internal/public/nodes.js
```

---

## 9. GitHub Actions

当前工作流目标：

- 手动触发全平台构建。
- 每个平台和架构单独构建二进制。
- artifact 直接上传单个二进制文件，避免双重压缩。
- Release 工作流使用唯一 tag，避免再次执行时覆盖上一次发布。
- 构建前执行 `go test ./...`，防止测试失败仍产物发布。

---

## 10. 后续优化方向

1. 继续把 `app.go` 拆分为配置、订阅、运行时、内核下载等独立文件。
2. 增加更多 Clash YAML 样本驱动测试。
3. 增强 `fallback` 策略的健康检查和 UI 状态展示。
4. 支持 Clash `proxy-providers` 远端拉取。
5. 为 Web UI 增加更细粒度的错误提示与配置校验。

---

## 11. 参考资料

- sing-box 文档（中文）：https://sing-box.sagernet.org/zh/configuration/
- sing-box 仓库：https://github.com/SagerNet/sing-box
- sing-box Releases：https://github.com/SagerNet/sing-box/releases
- v2rayN（解析兼容参考）：https://github.com/2dust/v2rayN
