package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/autoinst/AutoInstall/core"
	"github.com/autoinst/AutoInstall/packages"
)

var gitversion string

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
	instFile := "inst.json"
	var config core.InstConfig
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
		fmt.Println("AutoInstall-" + gitversion + " https://github.com/autoinst/AutoInstall")
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
		os.MkdirAll(".autoinst/cache", os.ModePerm)
		if config.Loader == "neoforge" {
			packages.NeoForgeB(core.InstConfig{})
		}
		if config.Loader == "forge" {
			var installerURL string
			if config.Download == "bmclapi" {
				installerURL = fmt.Sprintf(
					"https://bmclapi2.bangbang93.com/maven/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
					config.Version, config.LoaderVersion, config.Version, config.LoaderVersion,
				)
			} else {
				// 这里替换为官方源的URL，如果存在
				installerURL = fmt.Sprintf(
					"https://maven.minecraftforge.net/maven/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
					config.Version, config.LoaderVersion, config.Version, config.LoaderVersion,
				)
			}
			installerPath := filepath.Join("./.autoinst/cache", fmt.Sprintf("forge-%s-%s-installer.jar", config.Version, config.LoaderVersion))
			fmt.Println("当前为 forge 加载器，正在下载:", installerURL)
			if err := core.DownloadFile(installerURL, installerPath); err != nil {
				log.Println("下载 forge 失败:", err)
				return
			}
			fmt.Println("forge 安装器下载完成:", installerPath)

			// 提取 version.json
			versionInfo, err := core.ExtractVersionJson(installerPath)
			if err != nil {
				log.Println("提取 version.json 失败:", err)
				return
			}

			librariesDir := "./libraries"
			if err := packages.DownloadLibraries(versionInfo, librariesDir, config.MaxConnections); err != nil {
				log.Println("下载库文件失败:", err)
				return
			}

			if err := packages.DownloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
				log.Println("下载mc服务端失败:", err)
				return
			}

			fmt.Println("库文件下载完成")
			if err := core.RunInstaller(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download); err != nil {
				log.Println("运行安装器失败:", err)
			} else {
				// 检测是否存在 forge-版本-加载器版本-universal.jar
				universalJar := fmt.Sprintf("forge-%s-%s-universal.jar", config.Version, config.LoaderVersion)
				if _, err := os.Stat(universalJar); err == nil {
					// 创建 run.sh 文件
					runScriptPath := "run.sh"
					var runCommand string
					if simpfun {
						runCommand = fmt.Sprintf("/usr/bin/jdk/jdk1.8.0_361/bin/java -jar %s", universalJar)
					} else {
						runCommand = fmt.Sprintf("java -jar %s", universalJar)
					}
					// 写入 run.sh 文件
					if err := os.WriteFile(runScriptPath, []byte(runCommand), 0755); err != nil {
						log.Printf("无法创建 run.sh 文件: %v", err)
					} else {
						fmt.Println("已创建运行脚本:", runScriptPath)
					}
				} else {
					fmt.Printf("未找到文件 %s，跳过创建运行脚本。\n", universalJar)
				}
			}
		}
		if config.Loader == "fabric" {
			var installerURL string
			installerURL = "https://maven.fabricmc.net/net/fabricmc/fabric-installer/1.0.1/fabric-installer-1.0.1.jar"
			installerPath := filepath.Join("./.autoinst/cache", "fabric-installer-1.0.1.jar")
			fmt.Println("当前为 fabric 加载器，正在下载:", installerURL)
			if err := core.DownloadFile(installerURL, installerPath); err != nil {
				log.Println("下载 fabric 失败:", err)
				return
			}
			fmt.Println("fabric 安装器下载完成:", installerPath)
			librariesDir := "./libraries"
			if err := packages.DownloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
				log.Println("下载mc服务端失败:", err)
				return
			}
			fmt.Println("服务端下载完成")
			if err := core.RunInstaller(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download); err != nil {
				log.Println("运行安装器失败:", err)
				return
			}
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
			runCommand := fmt.Sprintf("%s -jar fabric-server-launch.jar", javaPath)
			// 写入 run.sh 文件
			if err := os.WriteFile(runScriptPath, []byte(runCommand), 0777); err != nil {
				log.Printf("无法创建 run.sh 文件: %v", err)
			} else {
				fmt.Println("已创建运行脚本:", runScriptPath)
			}
		}
		if config.Loader == "vanilla" {
			librariesDir := "./libraries"
			if err := packages.DownloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
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
