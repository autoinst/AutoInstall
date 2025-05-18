package pkg

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/autoinst/AutoInstall/core"
)

func FabricB(config core.InstConfig, simpfun bool, mise bool) {
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
	if err := core.RunInstaller(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download, simpfun, mise); err != nil {
		log.Println("运行安装器失败:", err)
		return
	}
	core.RunScript(config.Version, config.Loader, config.LoaderVersion, simpfun, mise)
}
