package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/autoinst/AutoInstall/core"
	"github.com/autoinst/AutoInstall/pkg"
)

var gitversion string

func Search() {
	mrpackFiles, err := filepath.Glob("modpack.mrpack")
	if err != nil {
		return
	}
	indexFiles, err := filepath.Glob("modrinth.index.json")
	if err != nil {
		return
	}
	zipFiles, err := filepath.Glob("modpack.zip")
	if err != nil {
		return
	}
	variablesFiles, err := filepath.Glob("variables.txt")
	if err != nil {
		return
	}

	allFiles := append(append(append(mrpackFiles, indexFiles...), zipFiles...), variablesFiles...)

	if len(allFiles) == 0 {
		fmt.Println("未找到整合包")
		return
	}
	for _, file := range mrpackFiles {
		fmt.Println("已有" + file)
		pkg.Modrinth(file)
	}
	for _, file := range indexFiles {
		fmt.Println("已有" + file)
		pkg.Modrinth(file)
	}
	for _, zipFile := range zipFiles {
		fmt.Println("发现其他整合包")
		pkg.SPCInstall(zipFile)
	}

	// 如果只有其他文件，则使用第一个
	if len(allFiles) == 1 {
		fmt.Println("发现整合包: " + allFiles[0])
		pkg.SPCInstall(allFiles[0])
	} else if len(allFiles) > 1 {
		fmt.Println("发现多个整合包，请手动指定")
		for _, file := range allFiles {
			fmt.Println("  " + file)
		}
	}
}

func main() {
	if gitversion == "" {
		gitversion = "NaN"
	}
	core.Argument(gitversion)
	os.MkdirAll(".autoinst/cache", os.ModePerm)
	instFile := "inst.json"
	var config core.InstConfig
	fmt.Println("AutoInstall-" + gitversion + " https://github.com/autoinst/AutoInstall")
	fmt.Println("正在扫描可用的整合包...")
	Search()
	if _, err := os.Stat(instFile); err == nil {
		data, err := os.ReadFile(instFile)
		if err != nil {
			log.Println("无法读取 inst.json 文件:", err)
			return
		}

		if err := json.Unmarshal(data, &config); err != nil {
			log.Println("无法解析 inst.json 文件:", err)
			return
		}
		fmt.Println("准备安装:")
		fmt.Printf("Minecraft版本: %s\n", config.Version)
		if config.Loader != "vanilla" {
			fmt.Printf("加载器: %s\n", config.Loader)
			fmt.Printf("加载器版本: %s\n", config.LoaderVersion)
			fmt.Println("\033[31m[警告] 加载器版本过新可能会无法正常下载\033[0m")
		}
		fmt.Printf("下载源: %s\n", config.Download)
		pkg.Common(config)
	} else if os.IsNotExist(err) {
		log.Println("inst.json 文件不存在")
	} else {
		log.Println("无法访问 inst.json 文件:", err)
	}
}
