# 数据库模块详解

## 技术选型

- **数据库**：SQLite（文件型，无需独立数据库服务）
- **ORM**：GORM v2
- **驱动**：`mattn/go-sqlite3`（CGO，需要 gcc）
- **WAL 模式**：提升并发读写性能，减少锁争用

## 连接配置

```go
dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on", dbPath)
```

| 参数 | 值 | 作用 |
|------|-----|------|
| `_journal_mode=WAL` | WAL | 写时不阻塞读，提升并发 |
| `_busy_timeout=5000` | 5000ms | 锁等待超时，避免 SQLITE_BUSY |
| `_foreign_keys=on` | on | 启用外键约束 |

## 数据表结构

### `settings` — 系统配置

键值对形式存储所有配置：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| key | string | 配置键（如 `webPort`、`webPath`） |
| value | string | 配置值 |

**常用配置键：**

| key | 默认值 | 说明 |
|-----|--------|------|
| `webPort` | `2095` | Web 面板端口 |
| `webPath` | `/app/` | Web 面板路径前缀 |
| `webListen` | `""` | 监听地址（空=0.0.0.0） |
| `subPort` | `2096` | 订阅服务端口 |
| `subPath` | `/sub/` | 订阅路径前缀 |
| `timeLocation` | `Asia/Tehran` | 时区 |
| `trafficAge` | `30` | 流量统计保留天数（0=不保留） |
| `config` | JSON | sing-box 基础配置（log/dns/route/experimental） |
| `secret` | 随机32字节 | Session 加密密钥 |

---

### `users` — 管理员账户

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| username | string | 用户名 |
| password | string | 密码（明文存储，待改进） |
| last_logins | string | 最近登录记录（JSON） |

首次启动自动创建 `admin/admin`。

---

### `tokens` — API Token

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| desc | string | 描述 |
| token | string | Token 值 |
| expiry | int64 | 过期时间（Unix 时间戳，0=永不过期） |
| user_id | uint | 关联用户（外键） |

Token 启动时加载到内存（`APIv2Handler.tokens`），避免每次请求查库。

---

### `tls` — TLS 证书配置

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| name | string | 证书名称 |
| server | JSON | 服务端 TLS 配置（inbound 侧） |
| client | JSON | 客户端 TLS 配置（outbound 侧） |

TLS 配置被 inbound 和 service 通过外键引用，支持多个 inbound 共用一份证书。

---

### `inbounds` — 入站规则

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| type | string | 协议类型（vmess/vless/trojan/hysteria2/…） |
| tag | string | 唯一标识（unique） |
| tls_id | uint | 外键 → tls 表 |
| addrs | JSON | 地址列表（多端口、域名） |
| out_json | JSON | 出站 JSON 模板（供客户端导出） |
| options | JSON（不持久化列）| 其他 sing-box 协议配置 |

**JSON 序列化设计**：`options` 字段在 GORM 中以 `blob` 存储，UnmarshalJSON/MarshalJSON 自定义实现：
- 存储时：从完整 JSON 中提取 `id/type/tag/tls_id/addrs/out_json`，剩余字段存入 `options`
- 读取时：将 `options` 展开合并回完整 JSON，供 sing-box 直接使用

---

### `outbounds` — 出站规则

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| type | string | 协议类型（direct/shadowsocks/vmess/…） |
| tag | string | 唯一标识（unique） |
| options | JSON（不持久化）| 协议配置 |

系统内置一个 `direct` 出站，不可删除。

---

### `services` — 服务

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| type | string | 服务类型（clash-api/v2ray-api/…） |
| tag | string | 唯一标识 |
| tls_id | uint | 外键 → tls 表 |
| options | JSON | 服务配置 |

---

### `endpoints` — 端点

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| type | string | 端点类型（wireguard/…） |
| tag | string | 唯一标识 |
| options | JSON | 端点配置 |
| ext | JSON | 扩展信息（如 WireGuard peers） |

---

### `clients` — 订阅客户端

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| enable | bool | 是否启用 |
| name | string | 客户端名称 |
| config | JSON | 协议配置（uuid/password/等） |
| inbounds | JSON | 绑定的入站 tag 列表 |
| links | JSON | 自定义分享链接 |
| volume | int64 | 流量限额（字节，0=不限） |
| expiry | int64 | 到期时间（Unix ms，0=不限） |
| down / up | int64 | 已用下行/上行流量（字节） |
| desc | string | 备注 |
| group | string | 分组标签 |
| delay_start | bool | 延迟启动（首次连接才开始计算） |
| auto_reset | bool | 自动重置流量 |
| reset_days | int | 重置周期（天） |
| next_reset | int64 | 下次重置时间 |
| total_up/down | int64 | 历史总流量 |

---

### `stats` — 流量统计

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint64 | 主键 |
| date_time | int64 | 时间戳（每10秒一条） |
| resource | string | 资源类型（`inbound`/`outbound`/`user`） |
| tag | string | 资源 tag |
| direction | bool | `true`=上行，`false`=下行 |
| traffic | int64 | 流量字节数 |

由 `StatsJob` 每 10 秒写入一次，`DelStatsJob` 按 `trafficAge` 清理旧数据。

---

### `changes` — 审计日志

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint64 | 主键 |
| date_time | int64 | 变更时间戳 |
| actor | string | 操作人（用户名） |
| key | string | 资源类型（`inbounds`/`clients`/…） |
| action | string | 操作（`new`/`edit`/`del`） |
| obj | JSON | 操作的对象内容 |

---

## 数据库迁移

版本迁移脚本位于 `cmd/migration/`：

| 文件 | 迁移内容 |
|------|---------|
| `1_1.go` | v1.0 → v1.1：clients 表新增字段 |
| `1_2.go` | v1.1 → v1.2：tls 表结构调整 |
| `1_3.go` | v1.2 → v1.3：clients 新增 delay_start/auto_reset |

触发方式：`./sui migrate`（CLI 命令）

---

## WAL 检查点

`WALCheckpointJob` 每 10 分钟执行一次 `PRAGMA wal_checkpoint(TRUNCATE)`，
将 WAL 文件内容合并回主数据库文件，控制 WAL 文件大小。
