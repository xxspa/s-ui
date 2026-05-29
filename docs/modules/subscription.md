# 订阅服务模块详解

## 概述

`sub/` 是一个**独立的 HTTP 服务器**（默认 `:2096`），专门处理客户端订阅请求，与主 Web 面板（`:2095`）完全隔离。

客户端用「订阅 ID」（`subid`）作为唯一凭证拉取配置，无需登录。

---

## 路由

```
GET /sub/{subid}              ← 默认格式（base64 链接列表）
GET /sub/{subid}?format=clash ← Clash YAML 格式
GET /sub/{subid}?format=json  ← sing-box JSON 格式
HEAD /sub/{subid}             ← 仅获取订阅头信息（不下载内容）
```

---

## 订阅格式

### Base64（默认）— `SubService`

文件：`subService.go`, `linkService.go`

```
vless://...
vmess://...
trojan://...
```
每行一个分享链接，整体 Base64 编码后返回。

**生成流程：**
1. 根据 `subid` 在 `clients` 表中找到对应客户端
2. 读取客户端绑定的入站列表（`client.Inbounds`）
3. 对每个入站，通过 `util.GenLink()` 生成对应协议的分享链接
4. 链接列表拼接后 Base64 编码

---

### Clash YAML — `ClashService`

文件：`clashService.go`

生成 Clash/Mihomo 兼容的 YAML 配置：

```yaml
proxies:
  - name: "节点名"
    type: vless
    server: example.com
    port: 443
    ...
proxy-groups:
  - name: "Proxy"
    type: select
    ...
rules:
  - MATCH,Proxy
```

支持通过 `settings.subClashExt` 设置自定义扩展（附加 rules、proxy-groups 等）。

---

### sing-box JSON — `JsonService`

文件：`jsonService.go`

生成客户端使用的完整 sing-box 配置：

```json
{
  "outbounds": [
    { "type": "vless", "tag": "节点名", ... },
    { "type": "direct", "tag": "direct" },
    { "type": "block", "tag": "block" },
    { "type": "dns", "tag": "dns-out" }
  ],
  "route": { ... }
}
```

支持通过 `settings.subJsonExt` 设置扩展配置。

---

## 订阅头信息

每次订阅响应都附带以下 HTTP 响应头：

| 头 | 格式 | 说明 |
|----|------|------|
| `Subscription-Userinfo` | `upload=x; download=y; total=z; expire=t` | 流量使用情况 |
| `Profile-Update-Interval` | `12`（小时） | 建议客户端更新间隔 |
| `Profile-Title` | 客户端名称 | 显示在客户端中的配置名 |

流量信息来自 `util.GetSubInfo(client)`，基于 client 的 `up/down/volume/expiry` 计算。

---

## 订阅 ID 设计

`subid` 存储在 `clients.config` 的 JSON 中（作为配置的一个字段），并非独立字段。

查找逻辑：遍历所有 clients，在其 config JSON 中匹配 `subid` 字段。

---

## 扩展配置（subExt）

通过 `settings` 表中的两个键自定义订阅扩展内容：

| 设置键 | 适用格式 | 说明 |
|--------|---------|------|
| `subClashExt` | Clash | 附加到 Clash YAML 的额外配置片段 |
| `subJsonExt` | JSON | 附加到 sing-box JSON 的额外字段 |

---

## 与外部订阅的互操作

`util/subToJson.go`：将外部订阅 URL（如其他机场的 base64 订阅）转换为 sing-box JSON 格式出站配置，供面板导入使用。

`util/linkToJson.go`：将单条分享链接（`vmess://`、`vless://`、`trojan://` 等）解析为 sing-box JSON 格式。
