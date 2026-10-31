package pkg

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/autoinst/AutoInstall/core"
)

func ForgeB(config core.InstConfig, simpfun bool, mise bool) error {
	core.ApplyConfigDefaults(&config)
	var err error
	config, err = resolveForgeTarget(config,
		func(retries int) (string, string, error) {
			return FetchLatestForgeVersion(retries, config.Download)
		},
		fetchForgeVersionsForMinecraft,
		func(mc string, retries int) (string, error) {
			return FetchLatestForgeLoaderForVersion(mc, retries, config.Download)
		},
	)
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

const forgePromotionsURL = "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json"

var forgePromotionVersionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)+$`)

func FetchLatestForgeVersion(maxRetries int, sources ...string) (string, string, error) {
	source := "official"
	if len(sources) > 0 {
		source = sources[0]
	}
	return fetchForgeVersion("", maxRetries, source)
}

func fetchForgeVersion(mcVersion string, maxRetries int, source string) (string, string, error) {
	official := func() (string, string, error) {
		var result struct {
			Promos map[string]string `json:"promos"`
		}
		if err := getJSONWithRetry(forgePromotionsURL, maxRetries, &result); err != nil {
			return "", "", err
		}
		return forgePromotionTarget(result.Promos, mcVersion)
	}
	mirror := func() (string, string, error) {
		if mcVersion == "" {
			return fetchLatestForgeVersionFromMirror(maxRetries)
		}
		versions, err := fetchForgeVersionsForMinecraft(mcVersion, maxRetries)
		if err != nil {
			return "", "", err
		}
		loader, err := latestForgeFromVersions(mcVersion, versions)
		return mcVersion, loader, err
	}
	primary, fallback := official, mirror
	if source == "bmclapi" {
		primary, fallback = mirror, official
	}
	mc, loader, primaryErr := primary()
	if primaryErr == nil {
		return mc, loader, nil
	}
	mc, loader, fallbackErr := fallback()
	if fallbackErr == nil {
		return mc, loader, nil
	}
	return "", "", fmt.Errorf("查询 Forge 版本失败: %w", errors.Join(primaryErr, fallbackErr))
}

func forgePromotionTarget(promos map[string]string, mcVersion string) (string, string, error) {
	if mcVersion != "" {
		loader := strings.TrimSpace(promos[mcVersion+"-latest"])
		if !forgePromotionVersionPattern.MatchString(loader) {
			return "", "", fmt.Errorf("Forge promotions 缺少 Minecraft %s 的有效 latest 版本", mcVersion)
		}
		return mcVersion, loader, nil
	}
	var latestMC, latestLoader string
	for name, value := range promos {
		if !strings.HasSuffix(name, "-latest") {
			continue
		}
		mc, loader := strings.TrimSuffix(name, "-latest"), strings.TrimSpace(value)
		if !forgePromotionVersionPattern.MatchString(mc) || !forgePromotionVersionPattern.MatchString(loader) {
			continue
		}
		if latestMC == "" || compareVersionTokens(mc, latestMC) > 0 {
			latestMC, latestLoader = mc, loader
		}
	}
	if latestMC == "" {
		return "", "", fmt.Errorf("Forge promotions 没有有效的 latest 版本")
	}
	return latestMC, latestLoader, nil
}

func fetchLatestForgeVersionFromMirror(maxRetries int) (string, string, error) {
	var result struct {
		Build struct {
			McVersion string `json:"mcversion"`
			Version   string `json:"version"`
		} `json:"build"`
	}
	if err := getJSONWithRetry("https://bmclapi2.bangbang93.com/forge/latest", maxRetries, &result); err != nil {
		return "", "", err
	}
	mc, loader := strings.TrimSpace(result.Build.McVersion), strings.TrimSpace(result.Build.Version)
	if mc == "" || loader == "" {
		return "", "", fmt.Errorf("BMCLAPI 最新 Forge 元数据缺少版本")
	}
	return mc, loader, nil
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

func FetchLatestForgeLoaderForVersion(mcVersion string, maxRetries int, sources ...string) (string, error) {
	source := "official"
	if len(sources) > 0 {
		source = sources[0]
	}
	if strings.TrimSpace(mcVersion) == "" {
		return "", fmt.Errorf("查询 Forge 版本时 Minecraft 版本不能为空")
	}
	_, loader, err := fetchForgeVersion(mcVersion, maxRetries, source)
	return loader, err
}

func latestForgeFromVersions(mc string, versions []string) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("没有找到 Minecraft 版本 %s 的 Forge 版本", mc)
	}
	sort.Slice(versions, func(i, j int) bool { return CompareForgeVersions(versions[i], versions[j]) > 0 })
	return versions[0], nil
}

func resolveForgeTarget(config core.InstConfig, latest func(int) (string, string, error), builds func(string, int) ([]string, error), latestForMinecraft func(string, int) (string, error)) (core.InstConfig, error) {
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
	if config.LoaderVersion == "latest" {
		loader, err := latestForMinecraft(config.Version, config.MaxRetries)
		if err != nil {
			return config, fmt.Errorf("获取 Minecraft %s 的最新 Forge 版本失败: %w", config.Version, err)
		}
		if strings.TrimSpace(loader) == "" {
			return config, fmt.Errorf("Minecraft %s 的最新 Forge 版本为空", config.Version)
		}
		config.LoaderVersion = loader
		return config, nil
	}
	if mcLatest {
		versions, err := builds(config.Version, config.MaxRetries)
		if err != nil {
			return config, fmt.Errorf("获取 Minecraft %s 的 Forge 版本失败: %w", config.Version, err)
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
