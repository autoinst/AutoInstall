package pkg

import (
	"fmt"
	"log"
	"os"

	"github.com/autoinst/AutoInstall/core"
)

func Common(config core.InstConfig) {
	javaPath, simpfun, mise := core.FindJava()
	if javaPath == "" {
		log.Println("未找到 Java，请确保已安装 Java 并设置 PATH。")
		return
	}
	fmt.Println("找到 Java 运行环境:", javaPath)
	if simpfun {
		fmt.Println("已启用 simpfun 环境")
	}
	if mise {
		fmt.Println("启用mise")
	}

	if config.Loader == "neoforge" {
		NeoForgeB(config, simpfun, mise)
	}
	if config.Loader == "forge" {
		ForgeB(config, simpfun, mise)
	}
	if config.Loader == "fabric" {
		FabricB(config, simpfun, mise)
	}
	if config.Loader == "vanilla" {
		librariesDir := "./libraries"
		if err := DownloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
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
}
