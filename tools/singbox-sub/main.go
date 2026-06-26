// singbox-sub: 从 sing-box 配置文件生成订阅文件
//
// 用法（生成指定用户）:
//
//	singbox-sub --config config.json --host example.com --user alice --out /var/www/alice.txt
//
// 用法（生成所有用户，每人一个文件，文件名=用户名）:
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

	"github.com/alireza0/s-ui/util"
)

func main() {
	var configPath, host, user, outFile, outDir string
	var noBase64 bool
	flag.StringVar(&configPath, "config", "", "sing-box 配置文件路径（必填）")
	flag.StringVar(&host, "host", "", "订阅链接使用的公网主机名（必填）")
	flag.StringVar(&user, "user", "", "用户名（对应 users[].name），空则输出所有用户")
	flag.StringVar(&outFile, "out", "", "输出文件路径（与 --out-dir 二选一）")
	flag.StringVar(&outDir, "out-dir", "", "输出目录，按用户名分别生成文件（与 --out 二选一）")
	flag.BoolVar(&noBase64, "no-base64", false, "输出明文链接，不做 base64 编码")
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

var supportedProto = map[string]bool{
	"vless": true, "vmess": true, "trojan": true,
	"shadowsocks": true, "hysteria2": true, "tuic": true,
	"socks": true, "http": true, "mixed": true,
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
		if !supportedProto[getString(inbound, "type")] {
			continue
		}
		tag := getString(inbound, "tag")
		port := int(getFloat(inbound, "listen_port"))
		tls := normalizeTLS(inbound)
		for _, u := range getUsers(inbound) {
			name := getString(u, "name")
			remark := tag
			if name != "" {
				remark = tag + "-" + name
			}
			links := genLinks(inbound, u, tls, host, port, remark)
			result[name] = append(result[name], links...)
		}
	}
	return result
}

func linksFromInbound(inbound map[string]interface{}, host, identifier string) []string {
	if !supportedProto[getString(inbound, "type")] {
		return nil
	}
	tag := getString(inbound, "tag")
	port := int(getFloat(inbound, "listen_port"))
	tls := normalizeTLS(inbound)

	var links []string
	for _, u := range getUsers(inbound) {
		if identifier != "" && !matchUser(u, identifier) {
			continue
		}
		name := getString(u, "name")
		remark := tag
		if name != "" {
			remark = tag + "-" + name
		}
		links = append(links, genLinks(inbound, u, tls, host, port, remark)...)
	}
	return links
}

// ---------- 链接生成：复用 util 包的 GetTransportParams / GetTlsParams / AddParams ----------

// buildAddr 构造单条地址记录，格式与 util.genLink 内部的 addrs 元素兼容。
func buildAddr(host string, port int, remark string, tls map[string]interface{}) map[string]interface{} {
	addr := map[string]interface{}{
		"server":      host,
		"server_port": float64(port),
		"remark":      remark,
	}
	if tls != nil {
		addr["tls"] = tls
	}
	return addr
}

func genLinks(inbound map[string]interface{}, user, tls map[string]interface{}, host string, port int, remark string) []string {
	proto := getString(inbound, "type")
	addr := buildAddr(host, port, remark, tls)

	// transport params：复用 util.GetTransportParams
	transportParams := util.GetTransportParams(inbound["transport"])

	switch proto {
	case "vless":
		return vlessLinks(user, inbound, tls, transportParams, addr, remark)
	case "vmess":
		return vmessLinks(user, inbound, tls, transportParams, addr, remark)
	case "trojan":
		return trojanLinks(user, tls, transportParams, addr, remark)
	case "shadowsocks":
		return shadowsocksLinks(user, inbound, host, port, remark)
	case "hysteria2":
		return hysteria2Links(user, inbound, tls, host, port, remark)
	case "tuic":
		return tuicLinks(user, inbound, tls, host, port, remark)
	case "socks":
		return socksLinks(user, host, port)
	case "http", "mixed":
		return httpLinks(user, tls, host, port)
	}
	return nil
}

func vlessLinks(user, inbound map[string]interface{}, tls map[string]interface{}, transportParams []util.LinkParam, addr map[string]interface{}, remark string) []string {
	uuid := getString(user, "uuid")
	if uuid == "" {
		return nil
	}
	params := make([]util.LinkParam, len(transportParams))
	copy(params, transportParams)
	if tls != nil {
		util.GetTlsParams(&params, tls, "allowInsecure")
		// flow 仅在 TCP 传输（无 transport 字段）时生效
		tr, _ := inbound["transport"].(map[string]interface{})
		if flow := getString(user, "flow"); flow != "" && getString(tr, "type") == "" {
			params = append(params, util.LinkParam{Key: "flow", Value: flow})
		}
	}
	uri := fmt.Sprintf("vless://%s@%s:%.0f", uuid, addr["server"], addr["server_port"])
	return []string{util.AddParams(uri, params, remark)}
}

func vmessLinks(user, inbound map[string]interface{}, tls map[string]interface{}, transportParams []util.LinkParam, addr map[string]interface{}, remark string) []string {
	uuid := getString(user, "uuid")
	if uuid == "" {
		return nil
	}
	obj := map[string]interface{}{
		"v":    "2",
		"ps":   remark,
		"add":  addr["server"],
		"port": fmt.Sprintf("%.0f", addr["server_port"]),
		"id":   uuid,
		"aid":  "0",
	}
	// transport params → vmess 字段映射
	net, typ, host, path := "tcp", "", "", ""
	for _, p := range transportParams {
		switch p.Key {
		case "type":
			net = p.Value
		case "host":
			host = p.Value
		case "path":
			path = p.Value
		}
	}
	if net == "http" {
		obj["net"] = "tcp"
		typ = "http"
	} else {
		obj["net"] = net
	}
	if typ != "" {
		obj["type"] = typ
	}
	if host != "" {
		obj["host"] = host
	}
	if path != "" {
		obj["path"] = path
	}
	// tls 部分：复用 GetTlsParams 解析后再映射到 vmess 字段
	if tls != nil {
		var tlsLinkParams []util.LinkParam
		util.GetTlsParams(&tlsLinkParams, tls, "allowInsecure")
		obj["tls"] = "tls"
		for _, p := range tlsLinkParams {
			switch p.Key {
			case "sni":
				obj["sni"] = p.Value
			case "fp":
				obj["fp"] = p.Value
			case "alpn":
				obj["alpn"] = p.Value
			}
		}
	} else {
		obj["tls"] = "none"
	}
	b, _ := json.Marshal(obj)
	return []string{"vmess://" + util.ToBase64(b)}
}

func trojanLinks(user map[string]interface{}, tls map[string]interface{}, transportParams []util.LinkParam, addr map[string]interface{}, remark string) []string {
	password := getString(user, "password")
	if password == "" {
		return nil
	}
	params := make([]util.LinkParam, len(transportParams))
	copy(params, transportParams)
	if tls != nil {
		util.GetTlsParams(&params, tls, "allowInsecure")
	}
	uri := fmt.Sprintf("trojan://%s@%s:%.0f", password, addr["server"], addr["server_port"])
	return []string{util.AddParams(uri, params, remark)}
}

func shadowsocksLinks(user, inbound map[string]interface{}, host string, port int, remark string) []string {
	method := getString(inbound, "method")
	var password string
	if strings.HasPrefix(method, "2022") {
		password = getString(inbound, "password") + ":" + getString(user, "password")
	} else {
		password = getString(user, "password")
		if password == "" {
			password = getString(inbound, "password")
		}
	}
	if method == "" || password == "" {
		return nil
	}
	encoded := util.ToBase64([]byte(method + ":" + password))
	return []string{fmt.Sprintf("ss://%s@%s:%d#%s", encoded, host, port, remark)}
}

func hysteria2Links(user, inbound map[string]interface{}, tls map[string]interface{}, host string, port int, remark string) []string {
	password := getString(user, "password")
	if password == "" {
		return nil
	}
	var params []util.LinkParam
	if tls != nil {
		util.GetTlsParams(&params, tls, "insecure")
	}
	if obfs, ok := inbound["obfs"].(map[string]interface{}); ok {
		if t := getString(obfs, "type"); t != "" {
			params = append(params, util.LinkParam{Key: "obfs", Value: t})
		}
		if p := getString(obfs, "password"); p != "" {
			params = append(params, util.LinkParam{Key: "obfs-password", Value: p})
		}
	}
	uri := fmt.Sprintf("hysteria2://%s@%s:%d", password, host, port)
	return []string{util.AddParams(uri, params, remark)}
}

func tuicLinks(user, inbound map[string]interface{}, tls map[string]interface{}, host string, port int, remark string) []string {
	uuid, password := getString(user, "uuid"), getString(user, "password")
	if uuid == "" || password == "" {
		return nil
	}
	var params []util.LinkParam
	if tls != nil {
		util.GetTlsParams(&params, tls, "insecure")
	}
	if cc := getString(inbound, "congestion_control"); cc != "" {
		params = append(params, util.LinkParam{Key: "congestion_control", Value: cc})
	}
	uri := fmt.Sprintf("tuic://%s:%s@%s:%d", uuid, password, host, port)
	return []string{util.AddParams(uri, params, remark)}
}

func socksLinks(user map[string]interface{}, host string, port int) []string {
	u, p := getString(user, "username"), getString(user, "password")
	if u != "" && p != "" {
		return []string{fmt.Sprintf("socks5://%s:%s@%s:%d", u, p, host, port)}
	}
	return []string{fmt.Sprintf("socks5://%s:%d", host, port)}
}

func httpLinks(user map[string]interface{}, tls map[string]interface{}, host string, port int) []string {
	scheme := "http"
	if tls != nil {
		scheme = "https"
	}
	u, p := getString(user, "username"), getString(user, "password")
	if u != "" && p != "" {
		return []string{fmt.Sprintf("%s://%s:%s@%s:%d", scheme, u, p, host, port)}
	}
	return []string{fmt.Sprintf("%s://%s:%d", scheme, host, port)}
}

// ---------- TLS 规格化：sing-box 服务端配置 → GetTlsParams 期望的格式 ----------

// normalizeTLS 把 sing-box 服务端 tls 字段转换成 util.GetTlsParams 能直接消费的格式：
//   - Reality: private_key → public_key（X25519 推导），short_id[] → short_id（取第一个）
//   - 标准 TLS: 字段名兼容，直接透传
func normalizeTLS(inbound map[string]interface{}) map[string]interface{} {
	raw, ok := inbound["tls"].(map[string]interface{})
	if !ok || !getBool(raw, "enabled") {
		return nil
	}

	tls := map[string]interface{}{
		"enabled":     true,
		"server_name": getString(raw, "server_name"),
		"insecure":    getBool(raw, "insecure"),
	}
	if alpn, ok := raw["alpn"]; ok {
		tls["alpn"] = alpn
	}
	if utls, ok := raw["utls"]; ok {
		tls["utls"] = utls
	}

	if reality, ok := raw["reality"].(map[string]interface{}); ok && getBool(reality, "enabled") {
		pubKey := getString(reality, "public_key")
		if pubKey == "" {
			pubKey = derivePublicKey(getString(reality, "private_key"))
		}
		shortID := ""
		if sids, ok := reality["short_id"].([]interface{}); ok && len(sids) > 0 {
			shortID, _ = sids[0].(string)
		}
		tls["reality"] = map[string]interface{}{
			"enabled":    true,
			"public_key": pubKey,
			"short_id":   shortID,
		}
	}

	return tls
}

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

// ---------- 辅助函数 ----------

func getUsers(inbound map[string]interface{}) []map[string]interface{} {
	for _, field := range []string{"users", "clients"} {
		raw, ok := inbound[field].([]interface{})
		if !ok {
			continue
		}
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
	// shadowsocks 单用户模式
	if method := getString(inbound, "method"); method != "" {
		if pass := getString(inbound, "password"); pass != "" {
			return []map[string]interface{}{{"password": pass, "name": ""}}
		}
	}
	return nil
}

func matchUser(user map[string]interface{}, id string) bool {
	for _, field := range []string{"name", "uuid", "username", "password"} {
		if getString(user, field) == id {
			return true
		}
	}
	return false
}

func getString(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func getBool(m map[string]interface{}, key string) bool {
	v, _ := m[key].(bool)
	return v
}

func getFloat(m map[string]interface{}, key string) float64 {
	v, _ := m[key].(float64)
	return v
}
