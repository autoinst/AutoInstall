package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/autoinst/AutoInstall/core"
	"github.com/autoinst/AutoInstall/pkg"
)

var gitversion string

func main() {
	if gitversion == "" {
		gitversion = "NaN"
	}
	if err := core.SetupLogger(); err != nil {
		fmt.Println("无法初始化日志系统:", err)
	}
	defer core.CloseLogger()

	cleaninst := core.Argument(gitversion)
	if err := os.MkdirAll(".autoinst/cache", os.ModePerm); err != nil {
		exitWithError("无法创建缓存目录", err)
	}
	instFile := "inst.json"
	config, instExists, err := loadConfig(instFile)
	if err != nil {
		exitWithError("无法读取 inst.json 文件", err)
	}

	core.Log("AutoInstall-" + gitversion + " https://github.com/autoinst/AutoInstall")
	packFound, err := pkg.Search(config.MaxConnections, config.Argsment, config.MaxRetries, config)
	if err != nil {
		exitWithError("整合包扫描安装失败", err)
	}
	if packFound {
		config, instExists, err = loadConfig(instFile)
		if err != nil {
			exitWithError("无法读取整合包生成的 inst.json 文件", err)
		}
	}
	if err := pkg.WaitDownloads(); err != nil {
		exitWithError("下载任务失败", err)
	}
	if !instExists {
		exitWithError("未找到可安装目标", fmt.Errorf("inst.json 文件不存在，且未发现整合包"))
	}

	logInstallConfig(config)
	if err := pkg.Common(config, cleaninst); err != nil {
		exitWithError("安装失败", err)
	}

	writeDownloadErrorSummary()
}

func loadConfig(instFile string) (core.InstConfig, bool, error) {
	var config core.InstConfig
	core.ApplyConfigDefaults(&config)
	data, err := os.ReadFile(instFile)
	if err != nil {
		if os.IsNotExist(err) {
			return config, false, nil
		}
		return config, false, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, true, err
	}
	core.ApplyConfigDefaults(&config)
	return config, true, nil
}

func logInstallConfig(config core.InstConfig) {
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
	core.Logf("单源最大尝试次数: %d\n", config.MaxRetries)
}

func exitWithError(message string, err error) {
	core.Log(message+":", err)
	writeDownloadErrorSummary()
	core.CloseLogger()
	os.Exit(128)
}

func writeDownloadErrorSummary() {
	if len(core.DownloadErrors) == 0 {
		return
	}
	core.Log("安装过程中出现错误，详情请查看日志。")
	core.WriteLogRaw("\n--- 错误汇总 ---\n")
	for _, e := range core.DownloadErrors {
		core.WriteLogRaw(fmt.Sprintf("URL: %s\nError: %v\nResponse: %s\n----------------\n", e.URL, e.Err, e.Response))
	}
}
