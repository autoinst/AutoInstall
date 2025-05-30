package pkg

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/autoinst/AutoInstall/core"
)

func DownloadServerJar(version, loader, librariesDir string) error {
	downloadURL := fmt.Sprintf("https://bmclapi2.bangbang93.com/version/%s/server", version)
	var serverFileName string

	if loader == "forge" {
		if version >= "1.20.4" {
			serverFileName = fmt.Sprintf("server-%s-bundled.jar", version)
		} else {
			serverFileName = fmt.Sprintf("server-%s.jar", version)
		}
	} else if loader == "fabric" || loader == "vanilla" {
		serverFileName = "server.jar"
	} else {
		serverFileName = fmt.Sprintf("server-%s.jar", version)
	}

	var serverPath string
	if loader == "fabric" || loader == "vanilla" {
		serverPath = filepath.Join(".", serverFileName)
	} else {
		serverPath = filepath.Join(librariesDir, "net", "minecraft", "server", version, serverFileName)
	}

	if err := os.MkdirAll(filepath.Dir(serverPath), os.ModePerm); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	if err := core.DownloadFile(downloadURL, serverPath); err != nil {
		return fmt.Errorf("无法下载服务端文件 %s: %v", serverPath, err)
	}
	fmt.Println("下载完成 Minecraft 服务端:", serverPath)
	return nil
}
