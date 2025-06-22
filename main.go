package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/autoinst/AutoInstall/core"
	"github.com/autoinst/AutoInstall/pkg"
)

var gitversion string

func Search() {
	mrpackFiles, _ := filepath.Glob("modpack.mrpack")
	indexFiles, _ := filepath.Glob("modrinth.index.json")
	zipFiles, _ := filepath.Glob("modpack.zip")
	variablesFiles, _ := filepath.Glob("variables.txt")

	allMrpacks, _ := filepath.Glob("*.mrpack")
	allZips, _ := filepath.Glob("*.zip")

	// 合并整合包文件
	allPacks := append(allMrpacks, allZips...)

	// 合并所有类型文件用于判断是否完全为空
	allFiles := append(append(append(mrpackFiles, indexFiles...), zipFiles...), variablesFiles...)

	// 输出已有文件信息
	for _, file := range mrpackFiles {
		fmt.Println("已有 " + file)
		pkg.Modrinth(file)
	}
	for _, file := range indexFiles {
		fmt.Println("已有 " + file)
		pkg.Modrinth(file)
	}
	for _, zipFile := range zipFiles {
		fmt.Println("已有 " + zipFile)
		pkg.SPCInstall(zipFile)
	}

	if len(allFiles) == 0 && len(allPacks) == 0 {
		fmt.Println("未找到整合包")
		return
	}

	if contains(allMrpacks, "modpack.mrpack") {
		fmt.Println("发现整合包: modpack.mrpack")
		pkg.Modrinth("modpack.mrpack")
		return
	} else if contains(allZips, "modpack.zip") {
		fmt.Println("发现整合包: modpack.zip")
		pkg.SPCInstall("modpack.zip")
		return
	}

	// 只有一个整合包
	if len(allPacks) == 1 {
		fmt.Println("发现整合包: " + allPacks[0])
		if filepath.Ext(allPacks[0]) == ".zip" {
			pkg.SPCInstall(allPacks[0])
		} else {
			pkg.Modrinth(allPacks[0])
		}
		return
	}

	// 多个整合包
	if len(allPacks) > 1 {
		fmt.Println("发现多个整合包，但未找到 modpack.zip 或 modpack.mrpack")
		fmt.Println("请将要使用的整合包重命名为 modpack.zip 或 modpack.mrpack 后重试")
		for _, file := range allPacks {
			fmt.Println("  " + file)
		}
		os.Exit(0)
	}
}
func contains(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

func main() {
	if gitversion == "" {
		gitversion = "NaN"
	}
	cleaninst := core.Argument(gitversion)
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
	} else if os.IsNotExist(err) {
		fmt.Println("inst.json 文件不存在")
	} else {
		fmt.Println("无法访问 inst.json 文件:", err)
	}
}
