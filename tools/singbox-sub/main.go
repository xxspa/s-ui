// singbox-sub: 从 sing-box 配置文件生成订阅文件
//
// 用法（指定用户）:
//
//	singbox-sub --config config.json --host example.com --user alice --out /var/www/alice
//
// 用法（所有用户，每人一个文件，文件名=users[].name）:
//
//	singbox-sub --config config.json --host example.com --out-dir /var/www/sub/
//
// 格式（--format）:
//
//	base64  默认，base64 编码的链接列表（通用订阅格式）
//	links   明文链接列表，每行一条
//	json    sing-box 客户端 JSON 配置
//	clash   Clash Meta YAML 配置
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var configPath, host, user, outFile, outDir, format string
	flag.StringVar(&configPath, "config", "", "sing-box 配置文件路径（必填）")
	flag.StringVar(&host, "host", "", "订阅链接使用的公网主机名（必填）")
	flag.StringVar(&user, "user", "", "用户名（users[].name），空则输出所有用户")
	flag.StringVar(&outFile, "out", "", "输出文件路径（与 --out-dir 二选一）")
	flag.StringVar(&outDir, "out-dir", "", "输出目录，按用户名生成文件（与 --out 二选一）")
	flag.StringVar(&format, "format", "base64", "输出格式：base64 / links / json / clash")
	flag.Parse()

	if configPath == "" || host == "" {
		flag.Usage()
		os.Exit(1)
	}
	if outFile == "" && outDir == "" {
		fmt.Fprintln(os.Stderr, "错误：请指定 --out 或 --out-dir")
		os.Exit(1)
	}
	if format != "base64" && format != "links" && format != "json" && format != "clash" {
		fmt.Fprintln(os.Stderr, "错误：--format 必须是 base64 / links / json / clash")
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
			path := filepath.Join(outDir, name)
			if err := writeOutput(path, links, format); err != nil {
				fmt.Fprintf(os.Stderr, "写入 %s 失败: %v\n", name, err)
			} else {
				fmt.Printf("生成 %s (%d 条链接)\n", path, len(links))
			}
		}
	} else {
		links := collectLinks(cfg, host, user)
		if err := writeOutput(outFile, links, format); err != nil {
			fmt.Fprintln(os.Stderr, "写入失败:", err)
			os.Exit(1)
		}
		fmt.Printf("生成 %s (%d 条链接)\n", outFile, len(links))
	}
}

func writeOutput(path string, links []string, format string) error {
	var content string
	switch format {
	case "base64":
		content = base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))
	case "links":
		content = strings.Join(links, "\n")
	case "json":
		outbounds, tags := linksToOutbounds(links)
		var err error
		content, err = formatSingboxJSON(outbounds, tags)
		if err != nil {
			return err
		}
	case "clash":
		outbounds, _ := linksToOutbounds(links)
		var err error
		content, err = formatClash(outbounds)
		if err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
