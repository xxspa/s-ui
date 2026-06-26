package main

// generator.go: 遍历 sing-box 配置里的 inbound，为每个用户生成订阅链接。
// 编排层：知道"做什么"，不知道"怎么转换格式"（那是 adapter.go 的事）。

import (
	"encoding/json"
	"os"
	"slices"

	"github.com/alireza0/s-ui/util"
)

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

// collectLinks 收集指定用户（identifier 为空则收集所有用户）的链接。
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

// collectByUser 返回 map[用户名][]链接，供 --out-dir 模式按用户分文件输出。
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

// linksFromInbound 针对单个 inbound，为匹配的用户生成所有订阅链接。
// 核心流程：适配格式 → 调用 util.LinkGenerator。
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
		// 每个用户独立一份 inbound，Tag 追加用户名作为链接备注
		inbound := *base
		if name, _ := user["name"].(string); name != "" {
			inbound.Tag = base.Tag + "-" + name
		}
		clientCfg, _ := json.Marshal(buildClientConfig(proto, user, raw))
		links = append(links, util.LinkGenerator(clientCfg, &inbound, host)...)
	}
	return links
}
