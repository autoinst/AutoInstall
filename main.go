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
	core.Log("AutoInstall-" + gitversion + " https://github.com/autoinst/AutoInstall")
	pkg.Search(config.MaxConnections, config.Argsment)
	if _, err := os.Stat(instFile); err == nil {
		data, err := os.ReadFile(instFile)
		if err != nil {
			core.Log("无法读取 inst.json 文件:", err)
			return
		}

		if err := json.Unmarshal(data, &config); err != nil {
			core.Log("无法解析 inst.json 文件:", err)
			return
		}
		core.Log("准备安装:")
		core.Logf("Minecraft版本: %s\n", config.Version)
		if config.Loader != "vanilla" {
			core.Logf("加载器: %s\n", config.Loader)
			core.Logf("加载器版本: %s\n", config.LoaderVersion)
			if config.Download == "bmclapi" {
				core.Log("\033[31m[警告] 加载器版本过新可能会无法正常下载\033[0m")
			}
		}
		core.Logf("下载源: %s\n", config.Download)
		pkg.Common(config, cleaninst)
		pkg.WaitDownloads()
	} else if os.IsNotExist(err) {
		core.Log("inst.json 文件不存在")
	} else {
		core.Log("无法访问 inst.json 文件:", err)
	}

	if len(core.DownloadErrors) > 0 {
		core.Log("安装过程中出现错误，详情请查看日志。")
		if core.LogFile != nil {
			core.LogFile.WriteString("\n--- 错误汇总 ---\n")
			for _, e := range core.DownloadErrors {
				core.LogFile.WriteString(fmt.Sprintf("URL: %s\nError: %v\nResponse: %s\n----------------\n", e.URL, e.Err, e.Response))
			}
		}
	}
}
