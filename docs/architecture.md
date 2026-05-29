# 项目整体架构

## 概述

s-ui 是一个 sing-box 代理内核的 Web 管理面板，分为三层：

```
┌─────────────────────────────────────────────────────────────────┐
│                         浏览器 / 客户端                          │
│              Vue 3 + Vuetify 3 (localhost:3000 开发)            │
└────────────────────────┬────────────────────────────────────────┘
                         │ HTTP API (/app/api, /app/apiv2)
┌────────────────────────▼────────────────────────────────────────┐
│                      Go 后端  :2095                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────────┐  │
│  │   api/   │  │ service/ │  │  core/   │  │   cronjob/     │  │
│  │ 路由层   │→ │ 业务层   │→ │sing-box  │  │ 定时任务       │  │
│  └──────────┘  └──────────┘  │ 内核封装 │  └────────────────┘  │
│                               └──────────┘                       │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │            database/ (SQLite + GORM)                     │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                         │ 独立 HTTP :2096
┌────────────────────────▼────────────────────────────────────────┐
│                      sub/ 订阅服务器                             │
│           输出 base64 / Clash YAML / sing-box JSON              │
└─────────────────────────────────────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────────────┐
│                   sing-box 内核（进程内）                         │
│        管理实际的 inbound/outbound 代理流量                      │
└─────────────────────────────────────────────────────────────────┘
```

---

## 技术栈

### 后端

| 组件 | 版本 | 用途 |
|------|------|------|
| Go | 1.26.3 | 主语言 |
| [gin-gonic/gin](https://github.com/gin-gonic/gin) | v1.12 | HTTP 框架 |
| [gorm.io/gorm](https://gorm.io) | v1.31 | ORM |
| [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) | v1.14 | SQLite（CGO） |
| [sagernet/sing-box](https://github.com/SagerNet/sing-box) | v1.13.12 | 代理内核 |
| [robfig/cron](https://github.com/robfig/cron) | v3 | 定时任务 |
| [op/go-logging](https://github.com/op/go-logging) | latest | 日志 |

### 前端

| 组件 | 版本 | 用途 |
|------|------|------|
| Vue 3 | 3.x | 框架 |
| Vuetify 3 | 3.x | UI 组件库 |
| Vite | 5.x | 构建工具 |
| TypeScript | 5.x | 类型系统 |
| vue-i18n | 9.x | 国际化（6 种语言） |
| Pinia | 2.x | 状态管理 |
| vue-router | 4.x | 路由 |

---

## 应用启动流程

```
main.go
  └── app.NewApp()
        ├── database.InitDB()          # 初始化 SQLite，AutoMigrate 所有表
        ├── setting.GetAllSetting()    # 加载设置到内存
        ├── core.NewCore()             # 创建 sing-box context（注册协议）
        ├── cronjob.NewCronJob()       # 创建定时任务调度器
        ├── web.NewServer()            # 创建 Web 服务器
        ├── sub.NewServer()            # 创建订阅服务器
        └── service.NewConfigService() # 创建配置服务（持有 core 引用）

  └── app.Start()
        ├── cronJob.Start()            # 启动定时任务
        ├── webServer.Start()          # 监听 :2095
        ├── subServer.Start()          # 监听 :2096
        └── configService.StartCore() # 读取 DB 配置 → 启动 sing-box
```

---

## 请求处理流程

### Web 面板请求

```
浏览器 GET /app/
  → web.Server (Gin)
  → embed.FS 返回 index.html (SPA 入口)

浏览器 POST /app/api/save
  → checkLogin 中间件（session 校验）
  → api.APIHandler.postHandler
  → api.ApiService.Save()
  → service.InboundService / OutboundService…
  → database (GORM)
  → 返回 JSON
  → 若需要重启内核：configService.RestartCore()
```

### 订阅请求

```
客户端 GET /sub/{subid}
  → sub.Server (Gin)
  → sub.SubHandler
  → ?format=clash  → ClashService.GetClash()
  → ?format=json   → JsonService.GetJson()
  → 默认           → SubService.GetSubs()  (base64)
  → 返回订阅内容 + Subscription-Userinfo 头
```

---

## 数据流：配置变更 → 内核重启

```
前端 POST /api/save (inbounds/outbounds/…)
  ↓
api.ApiService.Save(data, actor)
  ↓
service.*Service.Save(tx, act, data)   # 写入 SQLite
  ↓
记录 Changes 审计日志
  ↓
返回受影响的 inboundIds
  ↓
service.ConfigService.StartCore() 或 RestartInbounds()
  ↓
core.Stop() + 重新读取 DB → 组装 SingBoxConfig → core.Start()
  ↓
sing-box 内核用新配置重新监听端口
```

---

## 双服务器设计

| | Web 服务器 | 订阅服务器 |
|--|-----------|-----------|
| 包 | `web/` | `sub/` |
| 默认端口 | 2095 | 2096 |
| 路径前缀 | `/app/` | `/sub/` |
| 鉴权 | session cookie | 无（subid 即凭证） |
| TLS | 可选（独立证书） | 可选（独立证书） |
| 域名校验 | 可选 | 可选 |

两个服务器完全独立，可以分别配置不同端口、证书、域名。
