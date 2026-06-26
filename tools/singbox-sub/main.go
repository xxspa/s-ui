// singbox-sub: 从 sing-box 配置文件生成订阅文件
//
// 用法（指定用户）:
//
//	singbox-sub --config config.json --host example.com --user alice --out /var/www/alice.txt
//
// 用法（所有用户，每人一个文件）:
//
//	singbox-sub --config config.json --host example.com --out-dir /var/www/sub/
//
// 追加 --no-base64 输出明文链接。
package main

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"slices"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
)

func main() {
	var configPath, host, user, outFile, outDir string
	var noBase64 bool
	flag.StringVar(&configPath, "config", "", "sing-box 配置文件路径（必填）")
	flag.StringVar(&host, "host", "", "订阅链接使用的公网主机名（必填）")
	flag.StringVar(&user, "user", "", "用户名（users[].name），空则输出所有用户")
	flag.StringVar(&outFile, "out", "", "输出文件路径（与 --out-dir 二选一）")
	flag.StringVar(&outDir, "out-dir", "", "输出目录，按用户名生成文件（与 --out 二选一）")
	flag.BoolVar(&noBase64, "no-base64", false, "输出明文，不做 base64 编码")
	flag.Parse()

	if configPath == "" || host == "" {
		flag.Usage()
		os.Exit(1)
	}
	if outFile == "" && outDir == "" {
		fmt.Fprintln(os.Stderr, "错误：请指定 --out 或 --out-dir")
		os.Exit(1)
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取配置失败:", err)
		os.Exit(1)
	}

	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "创建目录失败:", err)
			os.Exit(1)
		}
		for name, links := range collectByUser(cfg, host) {
			if name == "" {
				continue
			}
			path := filepath.Join(outDir, name+".txt")
			if err := writeLinks(path, links, noBase64); err != nil {
				fmt.Fprintf(os.Stderr, "写入 %s 失败: %v\n", name, err)
			} else {
				fmt.Printf("生成 %s (%d 条链接)\n", path, len(links))
			}
		}
	} else {
		links := collectLinks(cfg, host, user)
		if err := writeLinks(outFile, links, noBase64); err != nil {
			fmt.Fprintln(os.Stderr, "写入失败:", err)
			os.Exit(1)
		}
		fmt.Printf("生成 %s (%d 条链接)\n", outFile, len(links))
	}
}

func writeLinks(path string, links []string, plain bool) error {
	content := strings.Join(links, "\n")
	if !plain {
		content = base64.StdEncoding.EncodeToString([]byte(content))
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// ---------- 配置解析 ----------

type singBoxConfig struct {
	Inbounds []json.RawMessage `json:"inbounds"`
}

func loadConfig(path string) (*singBoxConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg singBoxConfig
	return &cfg, json.Unmarshal(data, &cfg)
}

func collectLinks(cfg *singBoxConfig, host, identifier string) []string {
	var all []string
	for _, raw := range cfg.Inbounds {
		var inbound map[string]interface{}
		if json.Unmarshal(raw, &inbound) != nil {
			continue
		}
		all = append(all, linksFromInbound(inbound, host, identifier)...)
	}
	return all
}

func collectByUser(cfg *singBoxConfig, host string) map[string][]string {
	result := map[string][]string{}
	for _, raw := range cfg.Inbounds {
		var inbound map[string]interface{}
		if json.Unmarshal(raw, &inbound) != nil {
			continue
		}
		for _, user := range getUsers(inbound) {
			name, _ := user["name"].(string)
			links := linksFromInbound(inbound, host, name)
			result[name] = append(result[name], links...)
		}
	}
	return result
}

// ---------- 核心：加载进内存后直接复用 util.LinkGenerator ----------

func linksFromInbound(raw map[string]interface{}, host, identifier string) []string {
	proto, _ := raw["type"].(string)
	if !slices.Contains(util.InboundTypeWithLink, proto) {
		return nil
	}

	base := buildInbound(raw)

	var links []string
	for _, user := range getUsers(raw) {
		if identifier != "" && !matchUser(user, identifier) {
			continue
		}
		// 每个用户独立一份 inbound，Tag 带上用户名作为备注
		inbound := *base
		if name, _ := user["name"].(string); name != "" {
			inbound.Tag = base.Tag + "-" + name
		}
		clientCfg, _ := json.Marshal(buildClientConfig(proto, user, raw))
		links = append(links, util.LinkGenerator(clientCfg, &inbound, host)...)
	}
	return links
}

// buildInbound 把 sing-box 原始 inbound JSON 组装成 model.Inbound
// Options 存放 LinkGenerator 需要的协议字段（transport、listen_port、obfs 等）
func buildInbound(raw map[string]interface{}) *model.Inbound {
	inbound := &model.Inbound{
		Type:    raw["type"].(string),
		Tag:     raw["tag"].(string),
		Addrs:   json.RawMessage("null"), // 空则 LinkGenerator 用 hostname 自动填充
		OutJson: json.RawMessage("{}"),   // hysteria2/tuic 会访问此字段
	}

	if tlsRaw, ok := raw["tls"].(map[string]interface{}); ok {
		if enabled, _ := tlsRaw["enabled"].(bool); enabled {
			inbound.TlsId = 1
			inbound.Tls = buildModelTls(tlsRaw)
		}
	}

	// Options = 原始 JSON 去掉已单独处理的字段
	opts := cloneMap(raw)
	for _, k := range []string{"type", "tag", "tls", "users", "clients", "addrs", "out_json", "id"} {
		delete(opts, k)
	}
	inbound.Options, _ = json.Marshal(opts)

	return inbound
}

// buildModelTls 把 sing-box 服务端 tls 配置拆成 model.Tls{Server, Client}
// prepareTls（genLink.go 内部）会把两者合并后生成客户端链接参数
func buildModelTls(tlsRaw map[string]interface{}) *model.Tls {
	// Server：prepareTls 从中读取 enabled / server_name / alpn / reality.short_id[]
	serverTls := map[string]interface{}{
		"enabled":     tlsRaw["enabled"],
		"server_name": tlsRaw["server_name"],
	}
	if alpn, ok := tlsRaw["alpn"]; ok {
		serverTls["alpn"] = alpn
	}

	// Client：prepareTls 以此为基础，再叠加 Server 的字段
	// 不含 utls，避免生成空的 fp= 参数（服务端配置中无客户端指纹信息）
	clientTls := map[string]interface{}{
		"enabled":  false,
		"insecure": false,
	}

	if reality, ok := tlsRaw["reality"].(map[string]interface{}); ok {
		if enabled, _ := reality["enabled"].(bool); enabled {
			// Server 侧只保留 enabled 和 short_id 数组（prepareTls 会随机取一个）
			serverTls["reality"] = map[string]interface{}{
				"enabled":  true,
				"short_id": reality["short_id"],
			}
			// Client 侧需要 public_key（从 private_key 推导）
			pubKey, _ := reality["public_key"].(string)
			if pubKey == "" {
				privKey, _ := reality["private_key"].(string)
				pubKey = derivePublicKey(privKey)
			}
			clientTls["reality"] = map[string]interface{}{
				"enabled":    true,
				"public_key": pubKey,
				"short_id":   "", // 由 prepareTls 从 Server 的数组里取
			}
		}
	}

	serverJSON, _ := json.Marshal(serverTls)
	clientJSON, _ := json.Marshal(clientTls)
	return &model.Tls{
		Server: json.RawMessage(serverJSON),
		Client: json.RawMessage(clientJSON),
	}
}

// buildClientConfig 把 sing-box users[] 中的单个用户转成 util.LinkGenerator 期望的格式：
// map[协议名]map[字段]值，例如 {"vless": {"uuid": "...", "flow": "..."}}
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

// ---------- 辅助函数 ----------

// derivePublicKey 从 X25519 私钥（base64url 无填充）推导公钥
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
	// shadowsocks 单用户模式
	if method, _ := inbound["method"].(string); method != "" {
		if pass, _ := inbound["password"].(string); pass != "" {
			return []map[string]interface{}{{"password": pass, "name": ""}}
		}
	}
	return nil
}

func matchUser(user map[string]interface{}, id string) bool {
	for _, field := range []string{"name", "uuid", "username", "password"} {
		if v, _ := user[field].(string); v == id {
			return true
		}
	}
	return false
}

// pick 从 src 中提取指定 key，返回新 map
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
	json.Unmarshal(b, &result)
	return result
}
