// singbox-sub: 从 sing-box 配置文件生成订阅文件
//
// 用法（指定用户）:
//
//	singbox-sub --config config.json --host example.com --user alice --out /var/www/alice.txt
//
// 用法（所有用户，每人一个文件，文件名=users[].name）:
//
//	singbox-sub --config config.json --host example.com --out-dir /var/www/sub/
//
// 追加 --no-base64 输出明文链接。
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
