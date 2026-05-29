# CLAUDE.md — s-ui 项目指引

## 项目简介

s-ui 是基于 **sing-box** 内核的代理面板，提供 Web UI 管理入站/出站/客户端/订阅等资源。
后端：Go + Gin + GORM/SQLite；前端：Vue 3 + Vuetify 3 + Vite（独立 git 子模块）。

---

## 目录结构速览

```
s-ui/
├── main.go              # 入口，启动 APP 或执行 CLI 子命令
├── app/                 # 应用生命周期（组装所有模块）
├── api/                 # HTTP 路由层（session 鉴权 /api，token 鉴权 /apiv2）
├── service/             # 业务逻辑层（inbound/outbound/client/stats/config…）
├── core/                # sing-box 内核封装（启动/停止/流量追踪）
├── database/            # SQLite 初始化 + GORM 模型
├── cronjob/             # 定时任务（流量统计、客户端到期、WAL 检查点…）
├── sub/                 # 订阅服务器（独立端口，输出 base64/clash/json 订阅）
├── web/                 # Web 服务器 + embed 静态前端
├── middleware/          # Domain 校验中间件
├── network/             # AutoHTTPS 监听器
├── util/                # 链接生成/解析/base64/subInfo 工具
├── logger/              # 全局日志
├── config/              # 环境变量读取
├── cmd/                 # CLI 子命令（admin/setting/migration）
└── frontend/            # Vue 前端（git submodule → xxspa/s-ui-frontend）
```

详细说明见 [docs/](docs/)。

---

## 构建命令

### 开发构建（macOS arm64）

```bash
# 首次或后端代码变更后执行
CGO_ENABLED=1 go build \
  -tags "with_quic,with_grpc,with_utls,with_acme,with_gvisor,with_naive_outbound,badlinkname,with_tailscale" \
  -ldflags '-checklinkname=0' \
  -o sui main.go
```

> `web/html/` 目录只需存在文件即可编译（占位 index.html + 空 assets/ 已提交）。
> 不需要真实前端产物即可启动后端做接口开发。

### 生产构建（含前端）

```bash
./build.sh
# 等价于：
# cd frontend && npm install && npm run build && cd ..
# cp -R frontend/dist/* web/html/
# go build -tags "...with_musl..." -o sui main.go
```

### 仅构建前端

```bash
cd frontend
npm install   # 首次
npm run build
cp -R dist/* ../web/html/
```

---

## 运行 / 调试

### 直接运行

```bash
SUI_DEBUG=true SUI_DB_FOLDER=db ./sui
# Web UI: http://localhost:2095/app/  (默认账号 admin/admin)
# 订阅服务: http://localhost:2096/sub/
```

### 前端热重载开发（HMR）

```bash
# 终端1：保持后端运行
SUI_DEBUG=true SUI_DB_FOLDER=db ./sui

# 终端2：前端开发服务器（代理 /app/api → localhost:2095）
cd frontend && npm run dev
# 访问 http://localhost:3000/app/
```

### GoLand 调试配置

**Run/Debug Configurations → Go Build：**

| 字段 | 值 |
|------|----|
| Run kind | `File` |
| Files | `main.go` |
| Go tool arguments | `-tags "with_quic,with_grpc,with_utls,with_acme,with_gvisor,with_naive_outbound,badlinkname,with_tailscale" -ldflags "-checklinkname=0"` |
| Environment | `CGO_ENABLED=1;SUI_DEBUG=true;SUI_DB_FOLDER=db` |

---

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SUI_DEBUG` | `false` | 开启 debug 日志 + Gin debug 模式 |
| `SUI_LOG_LEVEL` | `info` | 日志级别（debug/info/warn/error） |
| `SUI_DB_FOLDER` | 二进制同目录/db | SQLite 数据库目录 |

---

## 关键约束

- **CGO 必须开启**：SQLite (`mattn/go-sqlite3`) 依赖 CGO。
- **`//go:embed *`**：`web/web.go` 嵌入整个 `web/html/` 目录，编译时该目录不能为空。
- **Build tags**：`with_musl` 仅用于 Linux 生产构建，macOS 开发去掉它。
- **子模块**：前端在 `frontend/`，指向 `xxspa/s-ui-frontend`，克隆时需 `git submodule update --init`。
- **双端口**：Web 面板默认 `:2095`，订阅服务默认 `:2096`，均可在设置中修改。

---

## 常用开发任务

```bash
# 新增 API 接口 → 修改 api/apiHandler.go + api/apiService.go
# 新增业务逻辑 → 在 service/ 下对应文件
# 新增定时任务 → cronjob/ 实现 Job 接口，在 cronJob.go 注册
# 修改数据模型 → database/model/，注意 GORM 自动迁移
# 修改前端页面 → frontend/src/views/ 或 components/
# 修改 API 类型 → frontend/src/types/ + 对应 .vue 同步
```
