package pkg

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/autoinst/AutoInstall/core"
)

func NeoForgeB(config core.InstConfig, simpfun bool, mise bool) error {
	core.ApplyConfigDefaults(&config)
	if config.Version == "latest" {
		latestVersion, err := FetchLatestNeoForgeVersion(config.MaxRetries)
		if err != nil {
			return fmt.Errorf("获取最新版本失败: %w", err)
		}
		config.LoaderVersion = latestVersion

		parts := strings.Split(latestVersion, ".")
		if len(parts) >= 3 {
			config.Version = fmt.Sprintf("1.%s.%s", parts[0], parts[1])
		} else {
			return fmt.Errorf("最新版本号格式不正确: %s", latestVersion)
		}
	}

	if config.LoaderVersion == "latest" {
		latestMatchingVersion, err := FetchLatestMatchingNeoForgeVersion(config.Version, config.MaxRetries)
		if err != nil {
			return fmt.Errorf("获取对应版本最新加载器版本失败: %w", err)
		}
		config.LoaderVersion = latestMatchingVersion
	}

	officialInstallerURL := fmt.Sprintf(
		"https://maven.neoforged.net/releases/net/neoforged/neoforge/%s/neoforge-%s-installer.jar",
		config.LoaderVersion, config.LoaderVersion,
	)
	installerURL, fallbackInstallerURL := downloadURLPair(officialInstallerURL, config.Download)
	installerPath := filepath.Join("./.autoinst/cache", fmt.Sprintf("neoforge-%s-installer.jar", config.LoaderVersion))
	core.Log("当前为 neoforge 加载器，正在下载:", installerURL)
	if err := core.DownloadFileWithFallback(installerURL, fallbackInstallerURL, installerPath, config.MaxRetries); err != nil {
		return fmt.Errorf("下载 neoforge 失败: %w", err)
	}
	core.Log("neoforge 安装器下载完成:", installerPath)

	versionInfo, err := core.ExtractVersionJson(installerPath)
	if err != nil {
		return fmt.Errorf("提取安装器元数据失败: %w", err)
	}

	librariesDir := "./libraries"
	if err := DownloadLibrariesAndServerJar(versionInfo, config.Version, config.Loader, librariesDir, config.MaxConnections, config.Download, config.MaxRetries); err != nil {
		return err
	}

	core.Log("库文件和服务端下载完成")
	if err := core.RunInstallerWithFallback(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download, simpfun, mise, config.MaxRetries, versionInfo.InstallFilePath); err != nil {
		return fmt.Errorf("运行安装器失败: %w", err)
	}
	if err := core.RunScript(config.Version, config.Loader, config.LoaderVersion, simpfun, mise, config.Argsment, versionInfo.InstallFilePath); err != nil {
		return fmt.Errorf("生成启动脚本失败: %w", err)
	}
	return nil
}

func FetchLatestNeoForgeVersion(maxRetries int) (string, error) {
	var result struct {
		Version string `json:"version"`
	}
	if err := getJSONWithRetry("https://maven.neoforged.net/api/maven/latest/version/releases/net/neoforged/neoforge", maxRetries, &result); err != nil {
		return "", err
	}
	return result.Version, nil
}

func FetchLatestMatchingNeoForgeVersion(mcVersion string, maxRetries int) (string, error) {
	var result struct {
		Versions []string `json:"versions"`
	}
	if err := getJSONWithRetry("https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge", maxRetries, &result); err != nil {
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

	sort.Slice(matchedVersions, func(i, j int) bool {
		return compareVersionTokens(matchedVersions[i], matchedVersions[j]) > 0
	})
	return matchedVersions[0], nil
}
