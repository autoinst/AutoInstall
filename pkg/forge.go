package pkg

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

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

	installerPath := filepath.Join("./.autoinst/cache", fmt.Sprintf("forge-%s-%s-installer.jar", config.Version, config.LoaderVersion))
	if err := downloadForgeInstaller(config, installerPath); err != nil {
		return err
	}
	core.Log("forge 安装器下载完成:", installerPath)

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

func downloadForgeInstaller(config core.InstConfig, installerPath string) error {
	urls := forgeInstallerURLs(config.Version, config.LoaderVersion)
	var lastErr error
	for i, officialURL := range urls {
		installerURL, fallbackInstallerURL := downloadURLPair(officialURL, config.Download)
		core.Log("当前为 forge 加载器，正在下载:", installerURL)
		if err := core.DownloadFileWithFallback(installerURL, fallbackInstallerURL, installerPath, config.MaxRetries); err != nil {
			lastErr = err
			if i+1 < len(urls) {
				core.Log("forge 安装器下载失败，尝试旧版命名:", err)
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("下载 forge 失败: %w", lastErr)
}

func forgeInstallerURLs(mcVersion, loaderVersion string) []string {
	primary := forgeInstallerURL(mcVersion, loaderVersion)
	legacyLoaderVersion := loaderVersion + "-" + mcVersion
	if strings.HasSuffix(loaderVersion, "-"+mcVersion) {
		return []string{primary}
	}
	return []string{primary, forgeInstallerURL(mcVersion, legacyLoaderVersion)}
}

func forgeInstallerURL(mcVersion, loaderVersion string) string {
	return fmt.Sprintf(
		"https://maven.minecraftforge.net/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
		mcVersion, loaderVersion, mcVersion, loaderVersion,
	)
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
