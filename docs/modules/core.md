# Core 模块详解（sing-box 内核封装）

## 概述

`core/` 是 s-ui 与 sing-box 内核的桥接层，负责：
1. 将数据库中的配置组装成 sing-box 可用的 JSON
2. 控制 sing-box 实例的启动/停止
3. 注入自定义的流量追踪器（stats、连接追踪）
4. 暴露运行时状态（在线连接、流量统计）

---

## 核心文件

### `main.go` — Core 结构体

```go
type Core struct {
    isRunning bool
    instance  *Box  // 当前运行的 sing-box 实例
}
```

`NewCore()` 创建全局 Context 并注册所有协议：

```go
globalCtx = sb.Context(globalCtx,
    InboundRegistry(),    // 所有入站协议
    OutboundRegistry(),   // 所有出站协议
    EndpointRegistry(),   // WireGuard endpoint 等
    DNSTransportRegistry(),
    ServiceRegistry(),    // Clash API、V2Ray API 等
)
```

| 方法 | 说明 |
|------|------|
| `Start(sbConfig []byte)` | 解析 JSON → 创建 Box → 启动 → 注册全局管理器 |
| `Stop()` | 关闭 Box 实例，清空所有管理器引用 |
| `IsRunning()` | 返回内核运行状态 |
| `GetInstance()` | 获取当前 Box 实例 |

---

### `box.go` — Box（sing-box 实例）

`Box` 是对 `sing-box` 内部所有组件的直接持有者，**基本等价于官方 `sing-box.Box`**，但额外注入了：
- `StatsTracker` — 流量统计追踪器
- `ConnTracker` — 连接在线追踪器

启动序列（4个阶段）：
```
Initialize → Start → PostStart → Started
```
每个阶段依次初始化：outbound → dnsTransport → dnsRouter → network → connection → router → inbound → endpoint → service

关闭顺序（反向）：
```
service → endpoint → inbound → outbound → router → connection → dns-router → dns-transport → network
```

---

### `register.go` / `register_naive.go` / `register_tailscale.go`

协议注册，通过 build tags 控制哪些协议编译进二进制：

| 文件 | Build Tag | 包含协议 |
|------|----------|---------|
| `register.go` | 无（始终编译） | vmess/vless/trojan/ss/hysteria/tuic/wireguard/tun/tproxy/socks/http/direct/dns/… |
| `register_naive.go` | `with_naive_outbound` | naive 协议 |
| `register_naive_stub.go` | `!with_naive_outbound` | 空注册（占位） |
| `register_tailscale.go` | `with_tailscale` | Tailscale 服务 |
| `register_tailscale_stub.go` | `!with_tailscale` | 空注册（占位） |

---

### `tracker_stats.go` — 流量统计追踪器

`StatsTracker` 实现 sing-box 的 `adapter.ConnectionTracker` 接口：

```go
type StatsTracker struct {
    stats map[string]*StatsCounter  // key: "inbound:tag" / "user:name" / "outbound:tag"
    mu    sync.RWMutex
}
```

每次新连接建立时，sing-box 通过 `router.AppendTracker()` 回调到此追踪器，累计上行/下行字节数。

`GetStats()` 读取并**重置**计数器（配合 StatsJob 每 10 秒调用一次，增量写库）。

追踪维度：
- `inbound:{tag}` — 某个入站的总流量
- `user:{name}` — 某个客户端用户的流量
- `outbound:{tag}` — 某个出站的流量

---

### `tracker_conn.go` — 连接在线追踪器

`ConnTracker` 追踪当前活跃连接，用于「在线用户」功能：

```go
type ConnTracker struct {
    conns map[string]net.Conn  // key: "inbound:tag" 或 "user:name"
    mu    sync.RWMutex
}
```

连接关闭时自动从 map 中移除，`GetOnlines()` 返回当前在线的资源列表。

---

### `outbound_check.go` — 出站可用性检测

`CheckOutbound(tag string)` — 通过指定出站发起 HTTP 请求，测量延迟：

```go
// 使用 sing-box 的 dialer，经过指定出站发起 TCP 连接
// 访问 https://www.gstatic.com/generate_204
// 返回延迟毫秒数或错误
```

对应前端「测速」功能。

---

### `endpoint.go` — Endpoint 注册

注册 WireGuard endpoint 类型到 sing-box context。

---

### `log.go` — 日志工厂

自定义 sing-box 日志输出，将 sing-box 内核日志接入 s-ui 的日志系统（`logger/`），而非直接写 stderr。

---

## 配置组装流程

`service.ConfigService.GetConfig()` 负责组装最终传给 core 的 JSON：

```
settings.config (基础 JSON)          ← 用户在面板「配置」页面编辑的内容
    + inbounds (from DB)              ← 注入每个 inbound 的 users 列表
    + outbounds (from DB)
    + services (from DB)
    + endpoints (from DB)
──────────────────────────────────────
→ 完整的 sing-box options JSON
→ core.Start(jsonBytes)
```

Inbound 注入用户：
```go
// 每个 inbound 对应的 clients 会被注入到 inbound 配置的 users 字段
// 只注入 enable=true 的客户端
// 支持各协议的用户格式（vmess uuid、trojan password 等）
```
