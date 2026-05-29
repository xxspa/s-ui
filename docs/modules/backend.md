# 后端模块详解

## `main.go` — 入口

```go
func main() {
    if len(os.Args) < 2 {
        runApp()   // 无参数 → 启动完整服务
    } else {
        cmd.ParseCmd()  // 有参数 → CLI 子命令
    }
}
```

两种运行模式：服务模式和 CLI 模式（见 `cmd/`）。

---

## `app/` — 应用生命周期

**文件：** `app.go`

`APP` 结构体组装所有顶层服务，是整个应用的"根"：

```go
type APP struct {
    service.SettingService
    configService *service.ConfigService
    webServer     *web.Server
    subServer     *sub.Server
    cronJob       *cronjob.CronJob
    core          *core.Core
}
```

| 方法 | 说明 |
|------|------|
| `Init()` | 初始化 DB、日志、core、cronJob、webServer、subServer |
| `Start()` | 按序启动所有服务，最后启动 sing-box 内核 |
| `Stop()` | 反序停止所有服务 |
| `RestartApp()` | Stop() + Start()，响应 SIGHUP 信号 |

信号处理：`SIGHUP` → 热重启；`SIGTERM` → 优雅退出。

---

## `config/` — 配置读取

**文件：** `config.go`

从**环境变量**读取运行时配置，不使用配置文件：

| 函数 | 环境变量 | 默认值 |
|------|----------|--------|
| `IsDebug()` | `SUI_DEBUG=true` | false |
| `GetLogLevel()` | `SUI_LOG_LEVEL` | info（debug 时强制 debug） |
| `GetDBFolderPath()` | `SUI_DB_FOLDER` | 二进制同目录/db |
| `GetVersion()` | — | 编译时 embed `config/version` 文件 |
| `GetName()` | — | 编译时 embed `config/name` 文件（值为 `s-ui`） |

---

## `database/` — 数据层

**文件：** `db.go`, `backup.go`, `model/`

### 初始化

```go
database.InitDB(path)  // 打开 SQLite，AutoMigrate，插入默认数据
```

- 使用 GORM + WAL 模式（`_journal_mode=WAL&_busy_timeout=5000`）
- 首次启动自动创建默认用户 `admin/admin`、默认 `direct` 出站

### 数据模型

详见 [database.md](database.md)。

### 备份/恢复

`backup.go` 提供：
- `GetDb()` — 返回 SQLite 文件的字节流（用于下载备份）
- `ImportDb()` — 替换当前数据库文件，之后重启应用

---

## `service/` — 业务逻辑层

所有 Service 都是**无状态结构体**，通过 `database.GetDB()` 获取连接，方便组合：

```go
type ConfigService struct {
    ClientService
    TlsService
    SettingService
    InboundService
    // ...
}
```

### 各 Service 职责

#### `SettingService`
- 从 `settings` 表读写键值对配置
- 常用设置有内存缓存（`GetAllSetting()`）
- 提供类型化 getter：`GetPort()`, `GetWebPath()`, `GetTimeLocation()` 等

#### `InboundService`
- CRUD 入站规则（`inbounds` 表）
- `GetAllConfig()` — 将 DB 中的入站配置组装为 sing-box JSON 格式
- `RestartInbounds(tx, ids)` — 局部重启指定入站（不重启整个内核）

#### `OutboundService`
- CRUD 出站规则（`outbounds` 表）
- 维护一个固定的 `direct` 出站（不可删除）

#### `ClientService`
- 管理订阅用户（`clients` 表）
- `DepleteClients()` — 检查流量/到期，禁用超限用户
- `GetClientConfig()` — 组装客户端的 sing-box 用户配置（注入到入站）
- 支持延迟启动（`delay_start`）、自动重置流量（`auto_reset`）

#### `TlsService`
- 管理 TLS 证书配置（`tls` 表）
- 提供 server/client 两侧的证书配置（JSON `RawMessage`）

#### `ConfigService`（核心服务）
- 持有 `*core.Core` 引用
- `GetConfig()` — 从 DB 读取基础配置 + 组装所有 inbound/outbound/service/endpoint 为完整 sing-box JSON
- `StartCore()` / `StopCore()` / `RestartCore()` — 控制内核生命周期
- 内置 15s 冷却时间防止频繁重启

#### `StatsService`
- `SaveStats()` — 从 `core.StatsTracker` 读取实时流量 → 写入 `stats` 表
- 维护内存中的在线资源列表（`onlineResources`）

#### `ServerService`
- 获取系统状态：CPU、内存、网络接口信息
- 获取内核运行时状态：在线连接数、uptime

#### `WarpService`
- 自动生成 Cloudflare Warp 账号
- 调用 Cloudflare API 注册设备、获取 WireGuard 密钥

---

## `api/` — HTTP 路由层

### 两套认证体系

| 路径 | 文件 | 认证方式 | 用途 |
|------|------|----------|------|
| `/app/api/*` | `apiHandler.go` | Session Cookie | 浏览器 Web UI |
| `/app/apiv2/*` | `apiV2Handler.go` | `Token` 请求头 | 外部程序/脚本 |

### API 路由设计

所有路由统一为两个入口：
```
POST /app/api/:postAction
GET  /app/api/:getAction
```

Action 通过 URL 参数传递（而非 RESTful 路径），优点是：路由规则极简，新增接口只需在 switch-case 里加一条。

### GET 接口列表

| action | 说明 |
|--------|------|
| `load` | 一次性加载所有配置数据（inbounds/outbounds/clients/…） |
| `inbounds/outbounds/…` | 按类型加载部分数据 |
| `settings` | 读取设置 |
| `stats` | 流量统计数据 |
| `status` | 服务器状态（CPU/内存/网络/内核运行时） |
| `onlines` | 当前在线的资源列表 |
| `logs` | sing-box 最近日志 |
| `changes` | 变更审计日志 |
| `keypairs` | 生成密钥对（WireGuard/Reality 等） |
| `getdb` | 下载数据库备份 |
| `tokens` | 获取 API Token 列表 |
| `singbox-config` | 获取当前完整 sing-box 配置 JSON |
| `checkOutbound` | 测试出站可用性（延迟检测） |

### POST 接口列表

| action | 说明 |
|--------|------|
| `login` | 登录（用户名/密码） |
| `changePass` | 修改密码 |
| `save` | 保存配置变更（统一入口，通过 JSON 区分资源类型） |
| `restartApp` | 重启整个应用 |
| `restartSb` | 仅重启 sing-box 内核 |
| `linkConvert` | 将分享链接转为 JSON |
| `subConvert` | 将订阅 URL 转为内部格式 |
| `importdb` | 导入数据库备份 |
| `addToken` / `deleteToken` | 管理 API Token |

### `ApiService` （`apiService.go`）

所有实际业务在此实现，同时被 v1/v2 Handler 复用：

```go
func (a *ApiService) Save(c *gin.Context, loginUser string) {
    // 解析 body 中的 Changes[] 数组
    // 每个 Change 有 obj（资源类型）和 acts（操作数组）
    // 分发给对应 Service.Save()，收集受影响的 inboundIds
    // 如需重启内核，调用 configService.RestartCore()
    // 记录审计日志到 changes 表
}
```

---

## `core/` — sing-box 内核封装

详见 [core.md](core.md)。

---

## `cronjob/` — 定时任务

详见 [cronjob.md](cronjob.md)。

---

## `sub/` — 订阅服务

详见 [subscription.md](subscription.md)。

---

## `web/` — Web 服务器

**文件：** `web.go`

```go
//go:embed *
var content embed.FS
```

将整个 `web/` 目录（包含 `html/`）编译进二进制。

- `html/index.html` → 作为 Go html/template 模板，注入 `BASE_URL` 变量
- `html/assets/` → 通过 `StaticFS` 提供静态文件，设置 1 年缓存
- 所有未匹配路由 → 返回 `index.html`（SPA 路由）
- 登录状态校验：未登录重定向到 `{base_url}login`

---

## `middleware/` — 中间件

**文件：** `domainValidator.go`

Domain 白名单校验：若配置了 `webDomain` 或 `subDomain`，请求的 `Host` 必须匹配，否则返回 403。

---

## `network/` — 网络工具

**文件：** `auto_https_conn.go`, `auto_https_listener.go`

`AutoHttpsListener`：在同一端口同时支持 HTTP 和 HTTPS。
通过检测首字节是否为 TLS ClientHello（`0x16`）来判断协议，自动路由到对应处理器。

---

## `util/` — 工具函数

| 文件 | 功能 |
|------|------|
| `base64.go` | Base64 URL-safe 编解码 |
| `genLink.go` | 从 inbound + client 配置生成分享链接（vmess://、vless://、trojan://…） |
| `linkToJson.go` | 解析分享链接 → sing-box JSON 格式 |
| `outJson.go` | 生成出站 JSON（供前端 OutJson 组件显示） |
| `subInfo.go` | 计算订阅头信息（用量/总量/到期） |
| `subToJson.go` | 将外部订阅 URL 内容转为内部 JSON 格式 |
| `common/` | 通用数组/错误/随机工具 |

---

## `cmd/` — CLI 子命令

```bash
./sui help           # 帮助
./sui admin reset    # 重置管理员密码
./sui setting list   # 查看所有设置
./sui setting set key value  # 修改设置
./sui migrate        # 执行数据库迁移
```

`migration/` 目录包含版本间的数据迁移脚本（1.1、1.2、1.3）。

---

## `logger/` — 日志

封装 `op/go-logging`，提供全局的 `logger.Info/Debug/Warning/Error` 函数。
日志级别由 `SUI_DEBUG` 和 `SUI_LOG_LEVEL` 环境变量控制。
