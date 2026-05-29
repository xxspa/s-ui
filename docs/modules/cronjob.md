# 定时任务模块详解

## 概述

`cronjob/` 使用 `robfig/cron` 实现后台定时任务，随应用启动，随应用停止。

---

## 任务列表

| 任务 | 执行频率 | 文件 |
|------|---------|------|
| `StatsJob` | 每 10 秒 | `statsJob.go` |
| `DepleteJob` | 每 1 分钟 | `depleteJob.go` |
| `DelStatsJob` | 每天 | `delStatsJob.go` |
| `CheckCoreJob` | 每 5 秒 | `checkCoreJob.go` |
| `WALCheckpointJob` | 每 10 分钟 | `WALCheckpointJob.go` |

---

## 各任务详解

### `StatsJob` — 流量统计持久化

**触发条件**：`trafficAge > 0`（设置中流量统计保留天数不为 0）时才保存到库，否则只更新内存在线状态。

```go
func (s *StatsJob) Run() {
    s.StatsService.SaveStats(s.enableTraffic)
}
```

流程：
1. 从 `core.StatsTracker.GetStats()` 读取并清零计数器
2. 遍历所有统计项，按 resource/tag/direction 累加到 `stats` 表
3. 更新 `clients` 表的 `up`/`down` 字段（用于流量限额检查）
4. 更新内存中的 `onlineResources`（在线入站/出站/用户列表）

---

### `DepleteJob` — 客户端到期/超限处理

每分钟检查所有客户端：

```go
func (s *DepleteJob) Run() {
    inboundIds, err := s.ClientService.DepleteClients()
    // 若有客户端被禁用，重启对应的入站
    s.InboundService.RestartInbounds(db, inboundIds)
}
```

`DepleteClients()` 逻辑：
- **流量超限**：`volume > 0 && (up + down) >= volume` → 设 `enable = false`
- **已到期**：`expiry > 0 && expiry < now` → 设 `enable = false`
- **到期自动重置**：`auto_reset = true && next_reset < now` → 重置 up/down，更新 next_reset
- **延迟启动**：`delay_start = true`，首次有流量时才开始计算（通过 StatsJob 检测）

被禁用的客户端所绑定的入站会被「最小重启」（只重启那些入站，不重启整个内核）以踢除连接。

---

### `DelStatsJob` — 旧统计数据清理

```go
// 删除 date_time < (now - trafficAge * 24h) 的 stats 记录
db.Where("date_time < ?", cutoff).Delete(&model.Stats{})
```

防止 stats 表无限增长。仅在 `trafficAge > 0` 时注册此任务。

---

### `CheckCoreJob` — 内核健康检查

每 5 秒检查一次 sing-box 内核是否在运行：

```go
func (s *CheckCoreJob) Run() {
    if !corePtr.IsRunning() {
        configService.StartCore()  // 内核异常退出时自动重启
    }
}
```

防止内核崩溃后无人发现，自动恢复服务。

---

### `WALCheckpointJob` — SQLite WAL 检查点

```go
db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
```

将 WAL 文件内容合并回主数据库文件并截断 WAL，防止 WAL 文件无限增长。
每 10 分钟执行一次，对性能影响极小。

---

## 时区配置

`CronJob.Start(loc, trafficAge)` 接收 `*time.Location` 参数，cron 任务按用户配置的时区执行（`@daily` 等相对时间基于该时区）。

```go
c.cron = cron.New(cron.WithLocation(loc), cron.WithSeconds())
```

---

## 扩展新任务

1. 在 `cronjob/` 下新建文件，实现 `cron.Job` 接口（即实现 `Run()` 方法）
2. 在 `cronJob.go` 的 `Start()` 中注册：

```go
c.cron.AddJob("@every 30m", NewMyJob())
```
