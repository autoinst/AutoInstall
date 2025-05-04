package packages

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/autoinst/AutoInstall/core"
)

func FabricB(config core.InstConfig, simpfun bool) {
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
	if err := DownloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
		log.Println("下载mc服务端失败:", err)
		return
	}
	fmt.Println("服务端下载完成")
	if err := core.RunInstaller(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download, simpfun); err != nil {
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
