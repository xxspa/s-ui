# 前端模块详解

## 技术栈

| 技术 | 版本 | 用途 |
|------|------|------|
| Vue 3 | 3.x | 核心框架（Composition API） |
| Vuetify 3 | 3.x | Material Design UI 组件库 |
| Vite | 5.x | 构建工具 + 开发服务器（HMR） |
| TypeScript | 5.x | 类型系统 |
| Pinia | 2.x | 状态管理 |
| vue-router | 4.x | 客户端路由 |
| vue-i18n | 9.x | 国际化（英/波斯/俄/越/中简/中繁） |

---

## 目录结构

```
frontend/src/
├── main.ts              # 应用入口，注册插件
├── App.vue              # 根组件
├── router/              # 路由配置
├── store/               # Pinia 状态管理
├── plugins/             # 插件注册（api、vuetify、i18n等）
├── types/               # TypeScript 类型定义
├── views/               # 页面级组件（对应路由）
├── components/          # 可复用 UI 组件
│   ├── protocols/       # 各协议的表单组件
│   ├── services/        # 服务类型组件
│   ├── tls/             # TLS 配置组件
│   ├── transports/      # 传输层配置组件
│   └── tiles/           # 仪表盘图表组件
├── layouts/             # 布局组件
│   ├── default/         # 主布局（AppBar、Drawer、View）
│   └── modals/          # 弹窗组件
├── locales/             # 国际化字符串
└── styles/              # 全局样式
```

---

## 路由（`router/index.ts`）

| 路径 | 视图 | 说明 |
|------|------|------|
| `/app/login` | `Login.vue` | 登录页 |
| `/app/` | `Home.vue` | 首页（状态仪表盘） |
| `/app/inbounds` | `Inbounds.vue` | 入站管理 |
| `/app/outbounds` | `Outbounds.vue` | 出站管理 |
| `/app/endpoints` | `Endpoints.vue` | 端点管理（WireGuard） |
| `/app/services` | `Services.vue` | 服务管理（Clash API 等） |
| `/app/clients` | `Clients.vue` | 客户端/订阅用户管理 |
| `/app/tls` | `Tls.vue` | TLS 证书管理 |
| `/app/dns` | `Dns.vue` | DNS 配置 |
| `/app/rules` | `Rules.vue` | 路由规则管理 |
| `/app/settings` | `Settings.vue` | 系统设置 |
| `/app/admins` | `Admins.vue` | 管理员账户 |
| `/app/basics` | `Basics.vue` | sing-box 基础配置（log/ntp/experimental） |

路由守卫：非登录页均检查登录态，未登录重定向到 `/app/login`。

---

## 状态管理（`store/`）

### `store/modules/data.ts` — 全局数据 Store

核心状态：

```typescript
{
  inbounds: Inbound[],
  outbounds: Outbound[],
  endpoints: Endpoint[],
  services: Service[],
  tls: Tls[],
  clients: Client[],
  config: SingBoxConfig,
  users: User[],
  settings: Settings,
  stats: Stats[],
  onlines: Onlines,
  status: ServerStatus,
}
```

所有数据通过 `GET /api/load` 一次性加载，局部操作后本地更新 store 并同步 `POST /api/save`。

---

## API 通信（`plugins/api.ts`）

封装了所有与后端的通信：

```typescript
// 统一的请求封装
httputil.get('/api/load')          → 加载所有数据
httputil.post('/api/save', data)   → 保存变更
httputil.get('/api/status')        → 获取服务器状态
httputil.get('/api/stats')         → 获取流量统计
```

`plugins/httputil.ts` 提供基础 fetch 封装，统一处理：
- Base URL（从 `window.BASE_URL` 读取，由后端模板注入）
- 401 未登录自动跳转
- 错误统一弹 toast

---

## 类型系统（`types/`）

TypeScript 类型与后端 Go 模型一一对应：

| 文件 | 对应 Go 模型 |
|------|------------|
| `types/inbounds.ts` | `model.Inbound` |
| `types/outbounds.ts` | `model.Outbound` |
| `types/clients.ts` | `model.Client` |
| `types/tls.ts` | `model.Tls` |
| `types/config.ts` | `service.SingBoxConfig` |
| `types/rules.ts` | sing-box 路由规则 |
| `types/dns.ts` | sing-box DNS 配置 |
| `types/endpoints.ts` | `model.Endpoint` |
| `types/services.ts` | `model.Service` |
| `types/transport.ts` | 传输层配置（ws/grpc/http） |
| `types/tls.ts` | TLS 配置 |
| `types/multiplex.ts` | Mux 复用配置 |
| `types/dial.ts` | 拨号配置 |
| `types/brutal.ts` | Hysteria Brutal 配置 |

---

## 组件体系

### 协议组件（`components/protocols/`）

每种代理协议对应一个 `.vue` 组件，负责渲染该协议的配置表单：

| 组件 | 协议 | 入站/出站 |
|------|------|---------|
| `Vless.vue` | VLESS | 两者 |
| `Vmess.vue` | VMess | 两者 |
| `Trojan.vue` | Trojan | 两者 |
| `Shadowsocks.vue` | Shadowsocks | 两者 |
| `Hysteria2.vue` | Hysteria2 | 两者 |
| `Tuic.vue` | TUIC | 两者 |
| `Wireguard.vue` | WireGuard | 两者 |
| `Naive.vue` | Naive | 入站 |
| `ShadowTls.vue` | ShadowTLS | 入站 |
| `Http.vue` | HTTP | 两者 |
| `Socks.vue` | SOCKS | 两者 |
| `Tun.vue` | TUN | 入站 |
| `TProxy.vue` | TProxy | 入站 |
| `Tailscale.vue` | Tailscale | 端点 |
| `Warp.vue` | Cloudflare Warp | 出站 |
| `Selector.vue` | 选择器 | 出站 |
| `UrlTest.vue` | 自动测速 | 出站 |
| `Direct.vue` | 直连 | 出站 |
| `Ssh.vue` | SSH | 出站 |
| `Tor.vue` | Tor | 出站 |

### 传输层组件（`components/transports/`）

| 组件 | 说明 |
|------|------|
| `WebSocket.vue` | WebSocket 配置 |
| `gRPC.vue` | gRPC 传输配置 |
| `Http.vue` | HTTP/2 传输配置 |
| `HttpUpgrade.vue` | HTTPUpgrade 传输配置 |

### TLS 组件（`components/tls/`）

| 组件 | 说明 |
|------|------|
| `InTLS.vue` | 入站 TLS 配置 |
| `OutTLS.vue` | 出站 TLS 配置 |
| `Acme.vue` | ACME 自动证书配置 |
| `Ech.vue` | ECH（加密 Client Hello）配置 |

### 弹窗组件（`layouts/modals/`）

每类资源的增删改弹窗：

| 弹窗 | 说明 |
|------|------|
| `Inbound.vue` | 入站编辑弹窗 |
| `Outbound.vue` / `OutboundBulk.vue` | 出站编辑/批量导入 |
| `Client.vue` / `ClientAddBulk.vue` / `ClientEditBulk.vue` | 客户端管理 |
| `Tls.vue` | TLS 证书弹窗 |
| `Endpoint.vue` | 端点编辑弹窗 |
| `Service.vue` | 服务编辑弹窗 |
| `Rule.vue` / `RuleImport.vue` | 路由规则 |
| `Ruleset.vue` / `RulesetImport.vue` | 规则集 |
| `Dns.vue` / `DnsRule.vue` | DNS 配置 |
| `Stats.vue` / `UsageStats.vue` | 流量统计图表 |
| `QrCode.vue` / `WgQrCode.vue` | 二维码显示 |
| `Backup.vue` | 备份/恢复 |
| `Changes.vue` | 审计日志 |
| `Logs.vue` | sing-box 日志 |
| `Admin.vue` | 管理员设置 |
| `Token.vue` | API Token 管理 |

---

## 国际化（`locales/`）

支持 6 种语言：

| 文件 | 语言 |
|------|------|
| `en.ts` | 英语 |
| `fa.ts` | 波斯语（Farsi） |
| `ru.ts` | 俄语 |
| `vi.ts` | 越南语 |
| `zhcn.ts` | 简体中文 |
| `zhtw.ts` | 繁体中文 |

语言切换存储在 localStorage，刷新后保持。

---

## 前端构建说明

```bash
# 开发模式（HMR，代理 /app/api → localhost:2095）
npm run dev

# 生产构建（输出到 dist/）
npm run build

# 代码检查
npm run lint
```

生产构建使用随机哈希文件名（`vite.config.mts` 中自定义 `getUniqueFileName`），防止浏览器缓存旧版本 JS/CSS。
