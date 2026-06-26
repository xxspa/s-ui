package main

// format.go: 把订阅链接转成 sing-box JSON 或 Clash YAML 格式。
//
// 核心流程：
//   links → util.GetOutbound → []outbound map → 组装 JSON/Clash
//
// sing-box JSON 格式：复用 sub/jsonService.go 的 defaultJson 模板结构，
//   拼 selector/urltest/direct 默认出站，加简单 route。
// Clash YAML 格式：从 outbound map 转 Clash proxy map，
//   逻辑参考 sub/clashService.go 的 ConvertToClashMeta，此处内联实现
//   以避免引入 sub 包的数据库依赖。

import (
	"encoding/json"
	"strings"

	"github.com/alireza0/s-ui/util"
	"gopkg.in/yaml.v3"
)

const defaultSingboxClientJSON = `{
  "inbounds": [
    {
      "type": "tun",
      "address": ["172.19.0.1/30", "fdfe:dcba:9876::1/126"],
      "mtu": 9000,
      "auto_route": true,
      "strict_route": false,
      "endpoint_independent_nat": false,
      "stack": "system",
      "platform": {
        "http_proxy": {
          "enabled": true,
          "server": "127.0.0.1",
          "server_port": 2080
        }
      }
    },
    {
      "type": "mixed",
      "listen": "127.0.0.1",
      "listen_port": 2080,
      "users": []
    }
  ]
}`

const basicClashConfig = `mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
external-controller: 127.0.0.1:9090
tun:
  enable: true
  stack: system
  auto-route: true
  auto-detect-interface: true
  dns-hijack:
    - any:53
dns:
  enable: true
  ipv6: false
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  default-nameserver:
    - 8.8.8.8
    - 1.1.1.1
  nameserver:
    - https://doh.pub/dns-query
    - https://1.0.0.1/dns-query
  fallback:
    - tcp://9.9.9.9:53
  fake-ip-filter:
    - "*.lan"
    - localhost
    - "*.local"
rules:
  - GEOIP,Private,DIRECT
  - MATCH,Proxy
`

// linksToOutbounds 把订阅链接列表转成 sing-box outbound map 列表。
// 复用 util.GetOutbound，不重复解析链接格式。
func linksToOutbounds(links []string) ([]map[string]interface{}, []string) {
	var outbounds []map[string]interface{}
	var tags []string
	tagNumEnable := 0
	if len(links) > 1 {
		tagNumEnable = 1
	}
	for i, link := range links {
		ob, tag, err := util.GetOutbound(link, (i+1)*tagNumEnable)
		if err == nil && len(tag) > 0 {
			outbounds = append(outbounds, *ob)
			tags = append(tags, tag)
		}
	}
	return outbounds, tags
}

// formatSingboxJSON 把 outbounds 组装成完整的 sing-box 客户端配置 JSON。
func formatSingboxJSON(outbounds []map[string]interface{}, tags []string) (string, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(defaultSingboxClientJSON), &cfg); err != nil {
		return "", err
	}

	defaults := []map[string]interface{}{
		{
			"type":      "selector",
			"tag":       "proxy",
			"outbounds": append([]string{"auto", "direct"}, tags...),
		},
		{
			"type":      "urltest",
			"tag":       "auto",
			"outbounds": tags,
			"url":       "http://www.gstatic.com/generate_204",
			"interval":  "10m",
			"tolerance": 50,
		},
		{"type": "direct", "tag": "direct"},
	}
	cfg["outbounds"] = append(defaults, outbounds...)
	cfg["route"] = map[string]interface{}{
		"auto_detect_interface": true,
		"final":                 "proxy",
		"rules": []interface{}{
			map[string]interface{}{"action": "sniff"},
			map[string]interface{}{"clash_mode": "Direct", "action": "route", "outbound": "direct"},
			map[string]interface{}{"clash_mode": "Global", "action": "route", "outbound": "proxy"},
		},
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	return string(out), err
}

// formatClash 把 outbounds 转成 Clash Meta YAML 配置。
// 逻辑参考 sub/clashService.go ConvertToClashMeta，内联实现避免引入 DB 依赖。
func formatClash(outbounds []map[string]interface{}) (string, error) {
	var proxies []interface{}
	var proxyTags []string

	for _, ob := range outbounds {
		t, _ := ob["type"].(string)
		if t == "selector" || t == "urltest" || t == "direct" {
			continue
		}

		proxy := map[string]interface{}{
			"name":   ob["tag"],
			"type":   t,
			"server": normalizeServer(ob["server"]),
			"port":   ob["server_port"],
		}

		switch t {
		case "vmess":
			proxy["uuid"] = ob["uuid"]
			if aid, ok := ob["alter_id"].(float64); ok {
				proxy["alterId"] = int(aid)
			} else {
				proxy["alterId"] = 0
			}
			proxy["cipher"] = "auto"
		case "vless":
			proxy["uuid"] = ob["uuid"]
			if flow, ok := ob["flow"].(string); ok && flow != "" {
				proxy["flow"] = flow
			}
		case "trojan":
			proxy["password"] = ob["password"]
		case "socks":
			proxy["type"] = "socks5"
			proxy["username"] = ob["username"]
			proxy["password"] = ob["password"]
		case "http":
			proxy["username"] = ob["username"]
			proxy["password"] = ob["password"]
		case "hysteria":
			proxy["auth-str"] = ob["auth_str"]
			if up, ok := ob["up_mbps"].(float64); ok {
				proxy["up"] = up
			}
			if down, ok := ob["down_mbps"].(float64); ok {
				proxy["down"] = down
			}
			if obfs, ok := ob["obfs"].(string); ok {
				proxy["obfs"] = obfs
			}
			addPortList(proxy, ob)
		case "hysteria2":
			proxy["password"] = ob["password"]
			if up, ok := ob["up_mbps"].(float64); ok {
				proxy["up"] = up
			}
			if down, ok := ob["down_mbps"].(float64); ok {
				proxy["down"] = down
			}
			if obfs, ok := ob["obfs"].(map[string]interface{}); ok {
				proxy["obfs"] = obfs["type"]
				proxy["obfs-password"] = obfs["password"]
			}
			addPortList(proxy, ob)
		case "tuic":
			proxy["uuid"] = ob["uuid"]
			proxy["password"] = ob["password"]
			if cc, ok := ob["congestion_control"].(string); ok {
				proxy["congestion-controller"] = cc
			}
		case "anytls":
			proxy["password"] = ob["password"]
			if tls, ok := ob["tls"].(map[string]interface{}); ok {
				proxy["sni"] = tls["server_name"]
				proxy["skip-cert-verify"] = tls["insecure"]
			}
		case "shadowsocks":
			proxy["type"] = "ss"
			proxy["cipher"] = ob["method"]
			proxy["password"] = ob["password"]
			if network, ok := ob["network"].(string); ok && network != "tcp" {
				proxy["udp"] = true
			}
		default:
			continue
		}

		// TLS
		tls, isTls := ob["tls"].(map[string]interface{})
		if isTls {
			if en, ok := tls["enabled"].(bool); ok && !en {
				isTls = false
			}
		}
		if isTls {
			proxy["tls"] = tls["enabled"]
			if reality, ok := tls["reality"].(map[string]interface{}); ok {
				if en, _ := reality["enabled"].(bool); en {
					ro := map[string]interface{}{}
					if pbk, ok := reality["public_key"].(string); ok {
						ro["public-key"] = pbk
					}
					if sid, ok := reality["short_id"].(string); ok {
						ro["short-id"] = sid
					}
					proxy["reality-opts"] = ro
				}
			}
			if utls, ok := tls["utls"].(map[string]interface{}); ok {
				if en, _ := utls["enabled"].(bool); en {
					if fp, ok := utls["fingerprint"].(string); ok {
						proxy["client-fingerprint"] = fp
					}
				}
			}
			if sni, ok := tls["server_name"].(string); ok {
				if t == "vless" || t == "vmess" {
					proxy["servername"] = sni
				} else {
					proxy["sni"] = sni
				}
			}
			if insecure, ok := tls["insecure"].(bool); ok && insecure {
				proxy["skip-cert-verify"] = true
			}
			switch t {
			case "hysteria", "hysteria2", "tuic":
				proxy["alpn"] = []string{"h3"}
			default:
				if alpn, ok := tls["alpn"].([]interface{}); ok {
					proxy["alpn"] = alpn
				}
			}
		}

		// Transport
		if transport, ok := ob["transport"].(map[string]interface{}); ok {
			tt, _ := transport["type"].(string)
			switch tt {
			case "ws", "httpupgrade":
				proxy["network"] = "ws"
				wsOpts := map[string]interface{}{}
				if path, ok := transport["path"].(string); ok {
					wsOpts["path"] = path
				}
				wsHeaders := map[string]interface{}{}
				if headers, ok := transport["headers"].(map[string]interface{}); ok {
					for k, v := range headers {
						if arr, ok := v.([]interface{}); ok && len(arr) > 0 {
							wsHeaders[k] = arr[0]
						} else {
							wsHeaders[k] = v
						}
					}
				}
				if _, hasHost := wsHeaders["Host"]; !hasHost {
					if host, ok := transport["host"].(string); ok && host != "" {
						wsHeaders["Host"] = host
					}
				}
				if len(wsHeaders) > 0 {
					wsOpts["headers"] = wsHeaders
				}
				if tt == "httpupgrade" {
					wsOpts["v2ray-http-upgrade"] = true
				}
				proxy["ws-opts"] = wsOpts
			case "grpc":
				proxy["network"] = "grpc"
				grpcOpts := map[string]interface{}{}
				if sn, ok := transport["service_name"].(string); ok {
					grpcOpts["grpc-service-name"] = sn
				}
				proxy["grpc-opts"] = grpcOpts
			case "http":
				httpOpts := map[string]interface{}{}
				if path, ok := transport["path"].(string); ok {
					httpOpts["path"] = path
				} else if paths, ok := transport["path"].([]interface{}); ok && len(paths) > 0 {
					httpOpts["path"] = paths[0]
				}
				if host, ok := transport["host"].([]interface{}); ok && len(host) > 0 {
					httpOpts["host"] = host[0]
				}
				if isTls {
					proxy["network"] = "h2"
					proxy["h2-opts"] = httpOpts
				} else {
					proxy["network"] = "http"
					proxy["http-opts"] = map[string]interface{}{"path": []interface{}{httpOpts["path"]}, "host": httpOpts["host"]}
				}
			}
		}

		proxies = append(proxies, proxy)
		if tag, ok := ob["tag"].(string); ok {
			proxyTags = append(proxyTags, tag)
		}
	}

	proxyGroups := []map[string]interface{}{
		{
			"name":     "Proxy",
			"type":     "select",
			"proxies":  append([]string{"Auto"}, proxyTags...),
		},
		{
			"name":      "Auto",
			"type":      "url-test",
			"proxies":   proxyTags,
			"url":       "http://www.gstatic.com/generate_204",
			"interval":  300,
			"tolerance": 50,
		},
	}

	var output map[string]interface{}
	if err := yaml.Unmarshal([]byte(basicClashConfig), &output); err != nil {
		return "", err
	}

	if p, ok := output["proxies"].([]interface{}); ok {
		output["proxies"] = append(p, proxies...)
	} else {
		output["proxies"] = proxies
	}

	pgIface := make([]interface{}, len(proxyGroups))
	for i, pg := range proxyGroups {
		pgIface[i] = pg
	}
	if pg, ok := output["proxy-groups"].([]interface{}); ok {
		output["proxy-groups"] = append(pg, pgIface...)
	} else {
		output["proxy-groups"] = pgIface
	}

	result, err := yaml.Marshal(output)
	return string(result), err
}

// normalizeServer 处理 IPv6 地址格式。
func normalizeServer(v interface{}) interface{} {
	s, ok := v.(string)
	if !ok {
		return v
	}
	if strings.Contains(s, ":") && !strings.Contains(s, ".") &&
		!(strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) {
		return "'[" + s + "]'"
	}
	return s
}

// addPortList 处理 hysteria/hysteria2 的多端口字段。
func addPortList(proxy map[string]interface{}, ob map[string]interface{}) {
	if portLists, ok := ob["server_ports"].([]interface{}); ok {
		var ports []string
		for _, p := range portLists {
			if portRange, ok := p.(string); ok {
				ports = append(ports, strings.ReplaceAll(portRange, ":", "-"))
			}
		}
		if len(ports) > 0 {
			proxy["ports"] = strings.Join(ports, ",")
		}
	}
}
