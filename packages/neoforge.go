package packages

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/autoinst/AutoInstall/core"
	"github.com/autoinst/AutoInstall/packages"
)

// NeoForgeB 函数用于安装 NeoForge 加载器
func NeoForgeB(config core.InstConfig) {
	var installerURL string
	if config.Download == "bmclapi" {
		installerURL = fmt.Sprintf(
			"https://bmclapi2.bangbang93.com/maven/net/neoforged/neoforge/%s/neoforge-%s-installer.jar",
			config.LoaderVersion, config.LoaderVersion,
		)
	} else {
		// 这里替换为官方源的URL，如果存在
		installerURL = fmt.Sprintf(
			"https://maven.neoforged.net/releases/net/neoforged/neoforge/%s/neoforge-%s-installer.jar",
			config.LoaderVersion, config.LoaderVersion,
		)
	}

	installerPath := filepath.Join("./.autoinst/cache", fmt.Sprintf("neoforge-%s-installer.jar", config.LoaderVersion))
	fmt.Println("当前为 neoforge 加载器，正在下载:", installerURL)
	if err := core.DownloadFile(installerURL, installerPath); err != nil {
		log.Println("下载 neoforge 失败:", err)
		return
	}
	fmt.Println("neoforge 安装器下载完成:", installerPath)

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
	if err := packages.RunInstaller(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download); err != nil {
		log.Println("运行安装器失败:", err)
	}
}
