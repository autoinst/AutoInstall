package pkg

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/autoinst/AutoInstall/core"
)

func ForgeB(config core.InstConfig, simpfun bool, mise bool) {
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
	if err := DownloadLibraries(versionInfo, librariesDir, config.MaxConnections); err != nil {
		log.Println("下载库文件失败:", err)
		return
	}

	if err := DownloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
		log.Println("下载mc服务端失败:", err)
		return
	}

	fmt.Println("库文件下载完成")
	if err := core.RunInstaller(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download, simpfun); err != nil {
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
