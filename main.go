package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/autoinst/AutoInstall/core"
	"github.com/autoinst/AutoInstall/pkg"
)

var gitversion string
var cfapiKey string

func main() {
	if gitversion == "" {
		gitversion = "NaN"
	}
	if err := core.SetupLogger(); err != nil {
		fmt.Println("无法初始化日志系统:", err)
	}
	defer core.CloseLogger()

	cleaninst := core.Argument(gitversion)
	core.Argument(gitversion)
	os.MkdirAll(".autoinst/cache", os.ModePerm)
	instFile := "inst.json"
	var config core.InstConfig
	fmt.Println("AutoInstall-" + gitversion + " https://github.com/autoinst/AutoInstall")
	pkg.Search(config.MaxConnections, config.Argsment)
	if _, err := os.Stat(instFile); err == nil {
		data, err := os.ReadFile(instFile)
		if err != nil {
			fmt.Println("无法读取 inst.json 文件:", err)
			return
		}

		if err := json.Unmarshal(data, &config); err != nil {
			fmt.Println("无法解析 inst.json 文件:", err)
			return
		}
		fmt.Println("准备安装:")
		fmt.Printf("Minecraft版本: %s\n", config.Version)
		if config.Loader != "vanilla" {
			fmt.Printf("加载器: %s\n", config.Loader)
			fmt.Printf("加载器版本: %s\n", config.LoaderVersion)
			if config.Download == "bmclapi" {
				fmt.Println("\033[31m[警告] 加载器版本过新可能会无法正常下载\033[0m")
			}
		}
		fmt.Printf("下载源: %s\n", config.Download)
		pkg.Common(config, cleaninst)
		pkg.WaitDownloads()
	} else if os.IsNotExist(err) {
		fmt.Println("inst.json 文件不存在")
	} else {
		fmt.Println("无法访问 inst.json 文件:", err)
	}

	if len(core.DownloadErrors) > 0 {
		fmt.Println("安装过程中出现错误，详情请查看日志。")
		if core.LogFile != nil {
			core.LogFile.WriteString("\n--- 错误汇总 ---\n")
			for _, e := range core.DownloadErrors {
				core.LogFile.WriteString(fmt.Sprintf("URL: %s\nError: %v\nResponse: %s\n----------------\n", e.URL, e.Err, e.Response))
			}
		}
	}
}
