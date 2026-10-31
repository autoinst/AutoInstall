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
	var err error
	config, err = resolveForgeTarget(config, FetchLatestForgeVersion, fetchForgeVersionsForMinecraft)
	if err != nil {
		return err
	}

	installerPath := filepath.Join("./.autoinst/cache", fmt.Sprintf("forge-%s-%s-installer.jar", config.Version, config.LoaderVersion))
	if err := downloadForgeInstaller(config, installerPath); err != nil {
		return err
	}
	core.Log("forge 安装器下载完成:", installerPath)

	return installForgeFamilyServer(config, installerPath, simpfun, mise)
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

func fetchForgeVersionsForMinecraft(mcVersion string, maxRetries int) ([]string, error) {
	url := fmt.Sprintf("https://bmclapi2.bangbang93.com/forge/minecraft/%s", mcVersion)
	var builds []struct {
		Version string `json:"version"`
	}
	if err := getJSONWithRetry(url, maxRetries, &builds); err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(builds))
	for _, build := range builds {
		if strings.TrimSpace(build.Version) != "" {
			versions = append(versions, build.Version)
		}
	}
	return versions, nil
}

func FetchLatestForgeLoaderForVersion(mcVersion string, maxRetries int) (string, error) {
	versions, err := fetchForgeVersionsForMinecraft(mcVersion, maxRetries)
	if err != nil {
		return "", err
	}
	return latestForgeFromVersions(mcVersion, versions)
}

func latestForgeFromVersions(mc string, versions []string) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("没有找到 Minecraft 版本 %s 的 Forge 版本", mc)
	}
	sort.Slice(versions, func(i, j int) bool { return CompareForgeVersions(versions[i], versions[j]) > 0 })
	return versions[0], nil
}

func resolveForgeTarget(config core.InstConfig, latest func(int) (string, string, error), builds func(string, int) ([]string, error)) (core.InstConfig, error) {
	mcLatest := config.Version == "latest"
	if mcLatest {
		mc, loader, err := latest(config.MaxRetries)
		if err != nil {
			return config, fmt.Errorf("获取最新 Forge 版本失败: %w", err)
		}
		if mc == "" || loader == "" {
			return config, fmt.Errorf("最新 Forge 元数据缺少版本")
		}
		config.Version = mc
		if config.LoaderVersion == "latest" {
			config.LoaderVersion = loader
			return config, nil
		}
	}
	if config.LoaderVersion == "latest" || mcLatest {
		versions, err := builds(config.Version, config.MaxRetries)
		if err != nil {
			return config, fmt.Errorf("获取 Minecraft %s 的 Forge 版本失败: %w", config.Version, err)
		}
		if config.LoaderVersion == "latest" {
			config.LoaderVersion, err = latestForgeFromVersions(config.Version, versions)
			return config, err
		}
		// A fixed loader is never replaced by a global latest build.
		for _, version := range versions {
			if config.LoaderVersion == version || config.LoaderVersion == version+"-"+config.Version {
				return config, nil
			}
		}
		return config, fmt.Errorf("Forge %s 不支持 Minecraft %s，保留固定加载器版本且拒绝安装", config.LoaderVersion, config.Version)
	}
	return config, nil
}

func CompareForgeVersions(a, b string) int {
	return compareVersionTokens(a, b)
}
