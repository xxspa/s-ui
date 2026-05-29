# s-ui 项目文档

s-ui 是基于 **sing-box v1.13** 内核的 Web 代理面板，提供完整的入站/出站/客户端/订阅管理功能。

---

## 文档目录

| 文档 | 内容 |
|------|------|
| [architecture.md](architecture.md) | 整体架构图、技术栈、启动流程、请求流程 |
| [development.md](development.md) | 本地开发、调试配置、新功能开发指引 |
| **模块详解** | |
| [modules/backend.md](modules/backend.md) | 所有 Go 后端模块（api/service/web/util/cmd/…） |
| [modules/core.md](modules/core.md) | sing-box 内核封装、流量追踪、协议注册 |
| [modules/database.md](modules/database.md) | 数据库设计、所有表结构、迁移机制 |
| [modules/cronjob.md](modules/cronjob.md) | 定时任务（统计/到期/清理/健康检查/WAL） |
| [modules/subscription.md](modules/subscription.md) | 订阅服务（base64/Clash/JSON 格式） |
| [modules/frontend.md](modules/frontend.md) | 前端架构、组件体系、类型系统、国际化 |

根目录的 [CLAUDE.md](../CLAUDE.md) 是为 Claude Code 准备的快速参考，包含构建命令、调试配置和开发约束。

---

## 快速开始

```bash
# 克隆（含前端子模块）
git clone git@github.com:xxspa/s-ui.git
cd s-ui
git submodule update --init --recursive

# 编译后端
CGO_ENABLED=1 go build \
  -tags "with_quic,with_grpc,with_utls,with_acme,with_gvisor,with_naive_outbound,badlinkname,with_tailscale" \
  -ldflags '-checklinkname=0' -o sui main.go

# 运行
SUI_DEBUG=true SUI_DB_FOLDER=db ./sui
# → http://localhost:2095/app/  (admin/admin)

# 前端热重载开发（另开终端）
cd frontend && npm install && npm run dev
# → http://localhost:3000/app/
```

---

## 架构一句话概括

```
Vue 3 前端（:3000 开发 / embed 到二进制生产）
    ↕ REST API
Go 后端（:2095）── sing-box 内核（进程内）
    ↕ 独立订阅服务（:2096）
SQLite 数据库（单文件，WAL 模式）
```
