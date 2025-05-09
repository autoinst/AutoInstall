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

	if len(mrpackFiles) == 0 && len(indexFiles) == 0 && len(zipFiles) == 0 && len(variablesFiles) == 0 {
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
}

func main() {
	if gitversion == "" {
		gitversion = "NaN"
	}
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Println("--help 获取帮助")
		fmt.Println("--version 获取版本")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("AutoInstall-" + gitversion)
		return
	}
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

		javaPath, simpfun := core.FindJava()
		if javaPath == "" {
			log.Println("未找到 Java，请确保已安装 Java 并设置 PATH。")
			return
		}
		fmt.Println("找到 Java 运行环境:", javaPath)
		if simpfun {
			fmt.Println("已启用 simpfun 环境")
		}

		if config.Loader == "neoforge" {
			pkg.NeoForgeB(config, simpfun)
		}
		if config.Loader == "forge" {
			pkg.ForgeB(config, simpfun)
		}
		if config.Loader == "fabric" {
			pkg.FabricB(config, simpfun)
		}
		if config.Loader == "vanilla" {
			librariesDir := "./libraries"
			if err := pkg.DownloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
				log.Println("下载mc服务端失败:", err)
				return
			}
			fmt.Println("服务端下载完成")
			// 创建 run.sh 文件
			runScriptPath := "run.sh"
			var javaPath string
			if simpfun {
				// 根据版本号选择 Java 路径
				if config.Version < "1.17" {
					javaPath = "/usr/bin/jdk/jdk1.8.0_361/bin/java"
				} else if config.Version >= "1.17" && config.Version <= "1.20.3" {
					javaPath = "/usr/bin/jdk/jdk-17.0.6/bin/java"
				} else {
					javaPath = "/usr/bin/jdk/jdk-21.0.2/bin/java"
				}
			} else {
				javaPath = "java"
			}
			// 拼接运行命令
			runCommand := fmt.Sprintf("%s -jar server.jar", javaPath)
			// 写入 run.sh 文件
			if err := os.WriteFile(runScriptPath, []byte(runCommand), 0777); err != nil {
				log.Printf("无法创建 run.sh 文件: %v", err)
			} else {
				fmt.Println("已创建运行脚本:", runScriptPath)
			}
		}
	} else if os.IsNotExist(err) {
		log.Println("inst.json 文件不存在")
	} else {
		log.Println("无法访问 inst.json 文件:", err)
	}
}
