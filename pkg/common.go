package pkg

import (
	"fmt"
	"log"

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
		core.RunScript(config.Version, config.Loader, config.LoaderVersion, simpfun, mise)
	}
}
