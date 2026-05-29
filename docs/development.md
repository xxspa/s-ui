# 开发指南

## 环境准备

| 工具 | 版本要求 | 说明 |
|------|---------|------|
| Go | ≥ 1.26 | 后端语言 |
| gcc / clang | 任意 | CGO 编译 SQLite 必须 |
| Node.js | ≥ 20 | 前端构建 |
| npm | ≥ 10 | 包管理 |
| git | 任意 | 含子模块支持 |

---

## 首次克隆

```bash
git clone git@github.com:xxspa/s-ui.git
cd s-ui
git submodule update --init --recursive  # 拉取前端子模块
```

---

## 日常开发工作流

### 纯后端开发（Go）

```bash
# 1. 编译后端（web/html/ 有占位文件即可，无需真实前端）
CGO_ENABLED=1 go build \
  -tags "with_quic,with_grpc,with_utls,with_acme,with_gvisor,with_naive_outbound,badlinkname,with_tailscale" \
  -ldflags '-checklinkname=0' \
  -o sui main.go

# 2. 运行（debug 模式）
SUI_DEBUG=true SUI_DB_FOLDER=db ./sui
```

修改 `.go` 文件后重新执行上面两步即可。

---

### 前端开发（Vue + HMR）

```bash
# 终端1：启动后端（仅需启动一次）
SUI_DEBUG=true SUI_DB_FOLDER=db ./sui

# 终端2：启动前端开发服务器
cd frontend
npm install    # 首次需要
npm run dev

# 访问 http://localhost:3000/app/
# 默认账号：admin / admin
```

修改 `.vue` / `.ts` 文件 → 浏览器**即时热更新**，无需重启后端。

---

### 全栈联调

后端改了 API 字段 → 同步修改 `frontend/src/types/` 中对应的 TypeScript 类型。

---

## GoLand 调试配置

**Run/Debug Configurations → Go Build：**

```
Run kind:         File
Files:            /path/to/s-ui/main.go
Go tool arguments: -tags "with_quic,with_grpc,with_utls,with_acme,with_gvisor,with_naive_outbound,badlinkname,with_tailscale" -ldflags "-checklinkname=0"
Environment:      CGO_ENABLED=1;SUI_DEBUG=true;SUI_DB_FOLDER=db
Working directory: /path/to/s-ui
```

配置好后直接点 🐛 Debug 按钮，所有断点正常工作。

---

## 新增功能开发模式

### 新增后端 API 接口

1. **`service/`** 新增或扩展 Service 方法（业务逻辑）
2. **`api/apiService.go`** 新增方法（HTTP 请求解析 → 调用 Service → 返回 JSON）
3. **`api/apiHandler.go`** 在 switch-case 中注册新 action

```go
// api/apiHandler.go
case "myNewAction":
    a.ApiService.MyNewAction(c)
```

### 新增数据模型

1. **`database/model/`** 新增 model struct（GORM 会自动迁移）
2. **`service/`** 新增对应 Service
3. 必要时在 `cmd/migration/` 写迁移脚本

### 新增定时任务

1. **`cronjob/`** 新建文件，实现 `Run()` 方法
2. **`cronjob/cronJob.go`** 注册任务

### 新增前端页面

1. **`frontend/src/views/`** 新建 `MyPage.vue`
2. **`frontend/src/router/index.ts`** 添加路由
3. **`frontend/src/layouts/default/Drawer.vue`** 添加侧边栏菜单项
4. **`frontend/src/locales/`** 所有语言文件添加文本

### 新增协议支持

1. **`core/register.go`** 中注册新协议
2. **`service/inbounds.go`** 或 `outbounds.go` 中添加该协议的配置组装逻辑
3. **`util/genLink.go`** 添加链接生成逻辑（如果有标准链接格式）
4. **`frontend/src/components/protocols/`** 新建协议表单组件
5. **`frontend/src/types/`** 添加 TypeScript 类型

---

## 数据库直接查看

```bash
# 用 sqlite3 CLI 查看开发数据库
sqlite3 db/s-ui.db

# 常用查询
.tables                          # 查看所有表
SELECT * FROM settings;          # 查看所有设置
SELECT id, name, enable FROM clients;  # 查看客户端列表
SELECT * FROM changes ORDER BY date_time DESC LIMIT 20;  # 查看最近变更
```

---

## 生产构建

```bash
./build.sh
```

等价于：
```bash
# 1. 构建前端
cd frontend
npm install
npm run build
cd ..

# 2. 复制前端产物
rm -fr web/html/*
cp -R frontend/dist/* web/html/

# 3. 构建后端（含所有 build tags）
BUILD_TAGS="with_quic,with_grpc,with_utls,with_acme,with_gvisor,with_naive_outbound,with_musl,badlinkname,tfogo_checklinkname0,with_tailscale"
go build -ldflags '-w -s -checklinkname=0 -extldflags "-Wl,-no_warn_duplicate_libraries"' \
  -tags "$BUILD_TAGS" -o sui main.go
```

> `with_musl` 仅 Linux 可用，macOS 去掉此 tag。

---

## 常见问题

### Q: 编译报错 `missing go sum entry`

```bash
go mod tidy
```

### Q: SQLite 编译失败 `cgo: C compiler not found`

```bash
# macOS
xcode-select --install

# Ubuntu
apt install gcc
```

### Q: 前端 `npm run dev` 代理失败（接口 404）

确认后端已启动且监听在 `:2095`：
```bash
lsof -i :2095
```

### Q: sing-box 内核启动失败

查看日志：
```bash
SUI_DEBUG=true ./sui 2>&1 | grep -i error
```
常见原因：端口占用、配置语法错误、权限不足（TUN 模式需要 root）。

### Q: 修改了前端子模块想提交

前端子模块是独立仓库，需要在 `frontend/` 目录下单独提交：
```bash
cd frontend
git add .
git commit -m "feat: xxx"
git push

# 然后回到主仓库更新子模块引用
cd ..
git add frontend
git commit -m "chore: update frontend submodule"
```
