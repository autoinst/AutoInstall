package pkg

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/autoinst/AutoInstall/core"
)

func FabricB(config core.InstConfig, simpfun bool, mise bool) {
	if config.Version == "latest" {
		latestRelease, err := FetchLatestFabricMinecraftVersion()
		if err != nil {
			core.Log("获取最新我的世界版本失败:", err)
			return
		}
		config.Version = latestRelease
		stableLoader, err := FetchLatestStableFabricLoaderVersion()
		if err != nil {
			core.Log("获取最新 Fabric Loader 版本失败:", err)
			return
		}
		config.LoaderVersion = stableLoader
	}
	if config.LoaderVersion == "latest" {
		stableLoader, err := FetchLatestStableFabricLoaderVersion()
		if err != nil {
			core.Log("获取最新 Fabric Loader 版本失败:", err)
			return
		}
		config.LoaderVersion = stableLoader
	}

	installerURL := "https://maven.fabricmc.net/net/fabricmc/fabric-installer/1.0.1/fabric-installer-1.0.1.jar"
	installerPath := filepath.Join("./.autoinst/cache", "fabric-installer-1.0.1.jar")
	core.Log("当前为 fabric 加载器，正在下载:", installerURL)
	if err := core.DownloadFile(installerURL, installerPath); err != nil {
		core.Log("下载 fabric 失败:", err)
		return
	}
	core.Log("fabric 安装器下载完成:", installerPath)

	librariesDir := "./libraries"
	if err := DownloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
		core.Log("下载 mc 服务端失败:", err)
		return
	}
	core.Log("服务端下载完成")

	if err := core.RunInstaller(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download, simpfun, mise); err != nil {
		core.Log("运行安装器失败:", err)
		return
	}
	core.RunScript(config.Version, config.Loader, config.LoaderVersion, simpfun, mise, config.Argsment)
}

func FetchLatestFabricMinecraftVersion() (string, error) {
	resp, err := http.Get("https://launchermeta.mojang.com/mc/game/version_manifest.json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Latest struct {
			Release string `json:"release"`
		} `json:"latest"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Latest.Release, nil
}

func FetchLatestStableFabricLoaderVersion() (string, error) {
	resp, err := http.Get("https://meta.fabricmc.net/v2/versions/loader")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var versions []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return "", err
	}

	for _, v := range versions {
		if v.Stable {
			return v.Version, nil
		}
	}

	return "", fmt.Errorf("未找到稳定版本的Fabric Loader")
}
