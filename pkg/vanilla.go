package pkg

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/autoinst/AutoInstall/core"
)

func DownloadServerJar(version, loader, librariesDir string) error {
	var serverFileName string
	var config core.InstConfig

	switch loader {
	case "forge":
		if version >= "1.20.4" {
			serverFileName = fmt.Sprintf("server-%s-bundled.jar", version)
		} else if version <= "1.16.5" {
			serverFileName = fmt.Sprintf("minecraft_server.%s.jar", version)
		} else {
			serverFileName = fmt.Sprintf("server-%s.jar", version)
		}
	case "fabric", "vanilla":
		serverFileName = "server.jar"
	default:
		serverFileName = fmt.Sprintf("server-%s.jar", version)
	}

	var serverPath string
	if loader == "fabric" || loader == "vanilla" {
		serverPath = filepath.Join(".", serverFileName)
	} else if loader == "forge" && version <= "1.16.5" {
		serverPath = filepath.Join(".", serverFileName)
	} else {
		serverPath = filepath.Join(librariesDir, "net", "minecraft", "server", version, serverFileName)
	}

	if err := os.MkdirAll(filepath.Dir(serverPath), os.ModePerm); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	var downloadURL string
	var err error

	if config.Download == "bmclapi" {
		downloadURL = fmt.Sprintf("https://bmclapi2.bangbang93.com/version/%s/server", version)
	} else {
		downloadURL, err = getOfficialServerJarURL(version)
		if err != nil {
			return fmt.Errorf("获取官方源失败: %v", err)
		}
	}

	if err := core.DownloadFile(downloadURL, serverPath); err != nil {
		return fmt.Errorf("无法下载服务端文件 %s: %v", serverPath, err)
	}

	core.Log("下载完成 Minecraft 服务端:", serverPath)
	return nil
}

func getOfficialServerJarURL(version string) (string, error) {
	resp, err := http.Get("https://launcher.mojang.com/mc/game/version_manifest.json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var manifest struct {
		Versions []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"versions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return "", err
	}

	var versionURL string
	for _, v := range manifest.Versions {
		if v.ID == version {
			versionURL = v.URL
			break
		}
	}

	if versionURL == "" {
		return "", fmt.Errorf("未找到版本 %s", version)
	}

	resp2, err := http.Get(versionURL)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()

	var versionInfo struct {
		Downloads struct {
			Server struct {
				URL string `json:"url"`
			} `json:"server"`
		} `json:"downloads"`
	}

	if err := json.NewDecoder(resp2.Body).Decode(&versionInfo); err != nil {
		return "", err
	}

	if versionInfo.Downloads.Server.URL == "" {
		return "", fmt.Errorf("未找到版本 %s 的服务端下载 URL", version)
	}

	return versionInfo.Downloads.Server.URL, nil
}
