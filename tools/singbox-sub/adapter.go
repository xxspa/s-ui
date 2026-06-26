package main

// adapter.go: 把 sing-box 原始 JSON 格式适配成 s-ui 内部格式。
//
// s-ui 的 util.LinkGenerator 需要：
//   - model.Inbound  — 协议类型、协议参数（Options）、TLS（Server/Client 分开）
//   - clientConfig   — {协议名: {凭证字段: 值}}
//
// sing-box 的 inbound JSON 是一个扁平对象，TLS 只有服务端视角，用户在 users[] 数组里。
// 本文件负责桥接两者，其余逻辑不在这里。

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"

	"github.com/alireza0/s-ui/database/model"
)

// buildInbound 把 sing-box inbound JSON 组装成 model.Inbound。
//
//   - Addrs 为空：LinkGenerator 会用传入的 hostname + listen_port 自动构造地址。
//   - OutJson 设为 "{}"：hysteria2/tuic 的链接生成器会访问此字段，需保证非 nil。
//   - Options 存放剩余字段（transport、listen_port、obfs 等），MarshalFull 会还原回 map。
func buildInbound(raw map[string]interface{}) *model.Inbound {
	inbound := &model.Inbound{
		Type:    raw["type"].(string),
		Tag:     raw["tag"].(string),
		Addrs:   json.RawMessage("null"),
		OutJson: json.RawMessage("{}"),
	}

	if tlsRaw, ok := raw["tls"].(map[string]interface{}); ok {
		if enabled, _ := tlsRaw["enabled"].(bool); enabled {
			inbound.TlsId = 1
			inbound.Tls = buildModelTls(tlsRaw)
		}
	}

	opts := cloneMap(raw)
	for _, k := range []string{"type", "tag", "tls", "users", "clients", "addrs", "out_json", "id"} {
		delete(opts, k)
	}
	inbound.Options, _ = json.Marshal(opts)

	return inbound
}

// buildModelTls 把 sing-box 服务端 TLS 配置拆成 model.Tls{Server, Client}。
//
// genLink.go 内部的 prepareTls 逻辑：以 Client 为基础，叠加 Server 的
// enabled/server_name/alpn/reality.short_id，最终生成客户端链接参数。
// 因此：
//   - Server：保留 enabled、server_name、alpn、reality.short_id[]
//   - Client：保留 insecure、reality.public_key（需从 private_key 推导）
func buildModelTls(tlsRaw map[string]interface{}) *model.Tls {
	serverTls := map[string]interface{}{
		"enabled":     tlsRaw["enabled"],
		"server_name": tlsRaw["server_name"],
	}
	if alpn, ok := tlsRaw["alpn"]; ok {
		serverTls["alpn"] = alpn
	}

	// 不含 utls：服务端配置没有客户端指纹信息，留空会生成噪音参数 fp=
	clientTls := map[string]interface{}{
		"enabled":  false,
		"insecure": false,
	}

	if reality, ok := tlsRaw["reality"].(map[string]interface{}); ok {
		if enabled, _ := reality["enabled"].(bool); enabled {
			serverTls["reality"] = map[string]interface{}{
				"enabled":  true,
				"short_id": reality["short_id"], // prepareTls 会从数组里随机取一个
			}
			pubKey, _ := reality["public_key"].(string)
			if pubKey == "" {
				privKey, _ := reality["private_key"].(string)
				pubKey = derivePublicKey(privKey)
			}
			clientTls["reality"] = map[string]interface{}{
				"enabled":    true,
				"public_key": pubKey,
				"short_id":   "", // prepareTls 负责从 Server 的数组里填入
			}
		}
	}

	serverJSON, _ := json.Marshal(serverTls)
	clientJSON, _ := json.Marshal(clientTls)
	return &model.Tls{
		Server: serverJSON,
		Client: clientJSON,
	}
}

// buildClientConfig 把 sing-box users[] 中的单个用户转成 util.LinkGenerator 期望的格式：
//
//	map[协议名]map[字段]值，例如 {"vless": {"uuid": "...", "flow": "..."}}
func buildClientConfig(proto string, user, inbound map[string]interface{}) map[string]map[string]interface{} {
	cfg := map[string]map[string]interface{}{}
	switch proto {
	case "vless":
		cfg["vless"] = pick(user, "uuid", "flow")
	case "vmess":
		cfg["vmess"] = pick(user, "uuid")
	case "trojan":
		cfg["trojan"] = pick(user, "password")
	case "shadowsocks":
		// 2022-blake3-aes-128-gcm 方法的密钥长度为 16 字节，genLink 用不同 key 区分
		key := "shadowsocks"
		if method, _ := inbound["method"].(string); method == "2022-blake3-aes-128-gcm" {
			key = "shadowsocks16"
		}
		cfg[key] = pick(user, "password")
	case "naive":
		cfg["naive"] = pick(user, "username", "password")
	case "hysteria":
		cfg["hysteria"] = map[string]interface{}{"auth_str": user["password"]}
	case "hysteria2":
		cfg["hysteria2"] = pick(user, "password")
	case "tuic":
		cfg["tuic"] = pick(user, "uuid", "password")
	case "anytls":
		cfg["anytls"] = pick(user, "password")
	case "socks", "mixed":
		cfg["socks"] = pick(user, "username", "password")
		cfg["http"] = cfg["socks"]
	case "http":
		cfg["http"] = pick(user, "username", "password")
	}
	return cfg
}

// derivePublicKey 从 X25519 私钥（base64url 无填充）推导对应公钥。
// sing-box 服务端配置只存 private_key，客户端链接需要 public_key。
func derivePublicKey(privB64 string) string {
	if privB64 == "" {
		return ""
	}
	b, err := base64.RawURLEncoding.DecodeString(privB64)
	if err != nil {
		return ""
	}
	priv, err := ecdh.X25519().NewPrivateKey(b)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())
}

// getUsers 从 inbound 中提取用户列表。
// sing-box 各协议用 "users" 或 "clients" 字段；shadowsocks 单用户模式密码在顶层。
func getUsers(inbound map[string]interface{}) []map[string]interface{} {
	for _, field := range []string{"users", "clients"} {
		if raw, ok := inbound[field].([]interface{}); ok {
			var users []map[string]interface{}
			for _, u := range raw {
				if m, ok := u.(map[string]interface{}); ok {
					users = append(users, m)
				}
			}
			if len(users) > 0 {
				return users
			}
		}
	}
	if method, _ := inbound["method"].(string); method != "" {
		if pass, _ := inbound["password"].(string); pass != "" {
			return []map[string]interface{}{{"password": pass, "name": ""}}
		}
	}
	return nil
}

// matchUser 按 name / uuid / username / password 匹配用户标识符。
func matchUser(user map[string]interface{}, id string) bool {
	for _, field := range []string{"name", "uuid", "username", "password"} {
		if v, _ := user[field].(string); v == id {
			return true
		}
	}
	return false
}

// pick 从 src 中提取指定 key，返回新 map。
func pick(src map[string]interface{}, keys ...string) map[string]interface{} {
	m := map[string]interface{}{}
	for _, k := range keys {
		if v, ok := src[k]; ok {
			m[k] = v
		}
	}
	return m
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	b, _ := json.Marshal(m)
	var result map[string]interface{}
	_ = json.Unmarshal(b, &result)
	return result
}
