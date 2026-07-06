# sub2socks5

`sub2socks5` 是一个基于 `Go + sing-box` 的本地 SOCKS5 代理管理器。它从订阅或手动输入解析节点，生成 sing-box 配置，并通过 Web UI 管理多端口 SOCKS5 服务、节点组、链式代理、DNS 与内核下载。

---

## 当前能力

### 订阅与节点

- 支持多个订阅地址，更新后自动合并节点。
- 支持 URI 订阅：`vmess`、`vless`、`trojan`、`ss`、`socks/socks5`、`hysteria2/hy2`、`tuic`。
- 支持标准 Clash/Mihomo YAML：读取 `proxies` 列表并转换为 sing-box 出站。
- 支持手动导入单行链接、多行文本、JSON 与表单节点。
- 对未知 Clash 类型启用兼容兜底，并使用 `[fallback]` 标记方便排查。

### SOCKS5 服务

- 可创建多个本地 SOCKS5 服务。
- 每个服务可以选择不同出口：单节点、节点组、链式代理或内置 `proxy/auto/block`。
- Web UI 自动分配端口并规避占用。

### 节点组策略

- `urltest`：使用 sing-box 原生 `urltest` 出站，按测试地址选择较优节点。
- `fallback`：使用 sing-box 原生 `selector` 结构表达固定默认出口，保留向后兼容的 UI 状态展示。
- 旧版非原生策略名会在配置加载时自动迁移为 `urltest` 并写回配置文件。

### 链式代理

- 可创建多条链式代理预设。
- 先添加的节点先经过；只有 SOCKS5 服务选择该链式代理后才会实际生效。

### DNS 与内核管理

- 支持 DoH 服务器预设与自定义。
- 支持 DoH 引导解析 DNS，默认 `1.1.1.1`。
- 支持检测系统架构、缓存 Release 列表、选择计划版本、下载并替换 sing-box 内核。

---

## 项目结构

- `D:\sub2socks5\main.go`：程序入口，嵌入 Web UI 静态资源。
- `D:\sub2socks5\internal\app\app.go`：后端 API、配置读写、运行时生命周期与内核管理。
- `D:\sub2socks5\internal\app\subscription_parser.go`：URI 与 Clash/Mihomo YAML 订阅解析。
- `D:\sub2socks5\internal\app\config_builder.go`：sing-box 配置生成、出站集合与节点组转换。
- `D:\sub2socks5\internal\public\`：Web UI 页面、脚本与样式。
- `D:\sub2socks5\.github\workflows\`：测试、构建与发布工作流。

运行时目录：

- `D:\sub2socks5\internal\data`：主配置、订阅状态、版本缓存。
- `D:\sub2socks5\internal\runtime`：生成的 `sing-box.json`。
- `D:\sub2socks5\internal\bin`：下载的 sing-box 内核。

启动时如果关键配置缺失，程序会自动生成默认配置，确保添加订阅或节点后可以立即保存并启动。

---

## 运行方式

```bash
go run .
```

默认 Web UI 监听：

- `http://0.0.0.0:18080`

如果默认端口被占用，程序会自动向后查找可用端口。

---

## 常规使用流程

1. 打开 Web UI。
2. 在基础设置里添加订阅地址或手动导入节点。
3. 点击“更新订阅”。
4. 在节点管理页维护节点、节点组或链式代理。
5. 在主页配置一个或多个 SOCKS5 服务及目标出口。
6. 保存配置；程序会自动生成 sing-box 配置。
7. 启动 sing-box 运行时。

---

## 测试与验证

### 单元测试

```bash
go test ./...
```

### 编译检查

```bash
go build ./...
```

### 前端语法检查

```bash
node --check internal/public/app.js
node --check internal/public/nodes.js
```

### API 检查

- `GET /api/config`
- `POST /api/subscription/refresh`
- `POST /api/runtime/start`
- `POST /api/runtime/stop`
- `GET /api/runtime/logs`
- `GET /api/nodes`
- `POST /api/nodes`

### SOCKS5 出口检查

假设某个服务监听 `127.0.0.1:18081`：

```bash
curl --socks5 127.0.0.1:18081 https://ifconfig.me
```

---

## GitHub Actions 打包

工作流支持手动触发，对多个平台和架构分别构建二进制文件，并把单个二进制作为 artifact / release asset 上传。

常用检查：

```bash
go test ./...
go build ./...
```

---

## 参考资料

- sing-box 文档（中文）：https://sing-box.sagernet.org/zh/configuration/
- sing-box 仓库：https://github.com/SagerNet/sing-box
- sing-box Releases：https://github.com/SagerNet/sing-box/releases
- v2rayN（解析兼容参考）：https://github.com/2dust/v2rayN
