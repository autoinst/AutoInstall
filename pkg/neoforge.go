package pkg

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/autoinst/AutoInstall/core"
)

func NeoForgeB(config core.InstConfig, simpfun bool, mise bool) {
	if config.Version == "latest" {
		latestVersion, err := FetchLatestNeoForgeVersion()
		if err != nil {
			core.Log("获取最新版本失败:", err)
			return
		}
		config.LoaderVersion = latestVersion

		parts := strings.Split(latestVersion, ".")
		if len(parts) >= 3 {
			config.Version = fmt.Sprintf("1.%s.%s", parts[0], parts[1])
		} else {
			core.Log("最新版本号格式不正确:", latestVersion)
			return
		}
	}

	if config.LoaderVersion == "latest" {
		latestMatchingVersion, err := FetchLatestMatchingNeoForgeVersion(config.Version)
		if err != nil {
			core.Log("获取对应版本最新加载器版本失败:", err)
			return
		}
		config.LoaderVersion = latestMatchingVersion
	}

	var installerURL string
	if config.Download == "bmclapi" {
		installerURL = fmt.Sprintf(
			"https://bmclapi2.bangbang93.com/maven/net/neoforged/neoforge/%s/neoforge-%s-installer.jar",
			config.LoaderVersion, config.LoaderVersion,
		)
	} else {
		installerURL = fmt.Sprintf(
			"https://maven.neoforged.net/releases/net/neoforged/neoforge/%s/neoforge-%s-installer.jar",
			config.LoaderVersion, config.LoaderVersion,
		)
	}

	installerPath := filepath.Join("./.autoinst/cache", fmt.Sprintf("neoforge-%s-installer.jar", config.LoaderVersion))
	core.Log("当前为 neoforge 加载器，正在下载:", installerURL)
	if err := core.DownloadFile(installerURL, installerPath); err != nil {
		core.Log("下载 neoforge 失败:", err)
		return
	}
	core.Log("neoforge 安装器下载完成:", installerPath)

	// 提取 version.json
	versionInfo, err := core.ExtractVersionJson(installerPath)
	if err != nil {
		core.Log("提取 version.json 失败:", err)
		return
	}

	librariesDir := "./libraries"
	if err := DownloadLibraries(versionInfo, librariesDir, config.MaxConnections, config.Download); err != nil {
		core.Log("下载库文件失败:", err)
		return
	}

	if config.Download == "bmclapi" {
		if err := DownloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
			core.Log("下载 mc 服务端失败:", err)
			return
		}
	}

	core.Log("库文件下载完成")
	if err := core.RunInstaller(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download, simpfun, mise); err != nil {
		core.Log("运行安装器失败:", err)
	}
	core.RunScript(config.Version, config.Loader, config.LoaderVersion, simpfun, mise, config.Argsment)
}

func FetchLatestNeoForgeVersion() (string, error) {
	resp, err := http.Get("https://maven.neoforged.net/api/maven/latest/version/releases/net/neoforged/neoforge")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Version string `json:"version"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Version, nil
}

func FetchLatestMatchingNeoForgeVersion(mcVersion string) (string, error) {
	resp, err := http.Get("https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Versions []string `json:"versions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	targetPrefix := strings.TrimPrefix(mcVersion, "1.") + "."

	var matchedVersions []string
	for _, v := range result.Versions {
		if strings.HasPrefix(v, targetPrefix) {
			matchedVersions = append(matchedVersions, v)
		}
	}

	if len(matchedVersions) == 0 {
		return "", fmt.Errorf("没有找到匹配 Minecraft 版本 %s 的 LoaderVersion", mcVersion)
	}

	return matchedVersions[len(matchedVersions)-1], nil
}
