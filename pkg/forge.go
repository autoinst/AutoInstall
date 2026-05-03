package pkg

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/autoinst/AutoInstall/core"
)

func ForgeB(config core.InstConfig, simpfun bool, mise bool) error {
	core.ApplyConfigDefaults(&config)
	if config.Version == "latest" {
		latestVersion, latestLoader, err := FetchLatestForgeVersion(config.MaxRetries)
		if err != nil {
			return fmt.Errorf("获取最新 Forge 版本失败: %w", err)
		}
		config.Version = latestVersion
		config.LoaderVersion = latestLoader
	}

	if config.LoaderVersion == "latest" {
		latestLoader, err := FetchLatestForgeLoaderForVersion(config.Version, config.MaxRetries)
		if err != nil {
			return fmt.Errorf("获取指定 Minecraft 版本的最新 Forge加载器 失败: %w", err)
		}
		config.LoaderVersion = latestLoader
	}

	officialInstallerURL := fmt.Sprintf(
		"https://maven.minecraftforge.net/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
		config.Version, config.LoaderVersion, config.Version, config.LoaderVersion,
	)
	installerURL, fallbackInstallerURL := downloadURLPair(officialInstallerURL, config.Download)
	installerPath := filepath.Join("./.autoinst/cache", fmt.Sprintf("forge-%s-%s-installer.jar", config.Version, config.LoaderVersion))
	core.Log("当前为 forge 加载器，正在下载:", installerURL)
	if err := core.DownloadFileWithFallback(installerURL, fallbackInstallerURL, installerPath, config.MaxRetries); err != nil {
		return fmt.Errorf("下载 forge 失败: %w", err)
	}
	core.Log("forge 安装器下载完成:", installerPath)

	versionInfo, err := core.ExtractVersionJson(installerPath)
	if err != nil {
		return fmt.Errorf("提取 version.json 失败: %w", err)
	}

	librariesDir := "./libraries"
	if err := DownloadLibraries(versionInfo, librariesDir, config.MaxConnections, config.Download, config.MaxRetries); err != nil {
		return fmt.Errorf("下载库文件失败: %w", err)
	}

	if err := DownloadServerJar(config.Version, config.Loader, librariesDir, config.Download, config.MaxRetries); err != nil {
		return fmt.Errorf("下载 mc 服务端失败: %w", err)
	}

	core.Log("库文件下载完成")
	if err := core.RunInstallerWithFallback(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download, simpfun, mise, config.MaxRetries); err != nil {
		return fmt.Errorf("运行安装器失败: %w", err)
	}
	if err := core.RunScript(config.Version, config.Loader, config.LoaderVersion, simpfun, mise, config.Argsment); err != nil {
		return fmt.Errorf("生成启动脚本失败: %w", err)
	}
	return nil
}

func FetchLatestForgeVersion(maxRetries int) (string, string, error) {
	var result struct {
		Build struct {
			McVersion string `json:"mcversion"`
			Version   string `json:"version"`
		} `json:"build"`
	}
	if err := getJSONWithRetry("https://bmclapi2.bangbang93.com/forge/latest", maxRetries, &result); err != nil {
		return "", "", err
	}
	return result.Build.McVersion, result.Build.Version, nil
}

func FetchLatestForgeLoaderForVersion(mcVersion string, maxRetries int) (string, error) {
	url := fmt.Sprintf("https://bmclapi2.bangbang93.com/forge/minecraft/%s", mcVersion)
	var builds []struct {
		Version string `json:"version"`
	}
	if err := getJSONWithRetry(url, maxRetries, &builds); err != nil {
		return "", err
	}

	if len(builds) == 0 {
		return "", fmt.Errorf("没有找到 Minecraft 版本 %s 的 Forge 版本", mcVersion)
	}

	sort.Slice(builds, func(i, j int) bool {
		return CompareForgeVersions(builds[i].Version, builds[j].Version) > 0
	})

	return builds[0].Version, nil
}

func CompareForgeVersions(a, b string) int {
	return compareVersionTokens(a, b)
}
