package pkg

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/autoinst/AutoInstall/core"
)

func NeoForgeB(config core.InstConfig, simpfun bool, mise bool) error {
	core.ApplyConfigDefaults(&config)
	var err error
	config, err = resolveNeoForgeTarget(config, FetchLatestNeoForgeVersion, FetchLatestMatchingNeoForgeVersion)
	if err != nil {
		return err
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

	return installForgeFamilyServer(config, installerPath, simpfun, mise)
}

func neoForgeMinecraftVersion(loader string) (string, error) {
	parts := strings.Split(loader, ".")
	if len(parts) < 3 {
		return "", fmt.Errorf("无法识别 NeoForge 版本: %s", loader)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", fmt.Errorf("无法识别 NeoForge 版本: %s", loader)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor < 0 || major < 20 {
		return "", fmt.Errorf("无法识别 NeoForge 版本: %s", loader)
	}
	if major >= 26 {
		return fmt.Sprintf("%d.%d", major, minor), nil
	}
	if minor == 0 {
		return fmt.Sprintf("1.%d", major), nil
	}
	return fmt.Sprintf("1.%d.%d", major, minor), nil
}

func resolveNeoForgeTarget(config core.InstConfig, latest func(int) (string, error), matching func(string, int) (string, error)) (core.InstConfig, error) {
	if config.Version == "latest" {
		loader, err := latest(config.MaxRetries)
		if err != nil {
			return config, fmt.Errorf("获取最新 NeoForge 版本失败: %w", err)
		}
		mc, err := neoForgeMinecraftVersion(loader)
		if err != nil {
			return config, err
		}
		config.Version = mc
		if config.LoaderVersion == "latest" {
			config.LoaderVersion = loader
		}
	}
	if config.LoaderVersion == "latest" {
		loader, err := matching(config.Version, config.MaxRetries)
		if err != nil {
			return config, fmt.Errorf("获取对应版本最新 NeoForge 加载器失败: %w", err)
		}
		config.LoaderVersion = loader
	}
	mc, err := neoForgeMinecraftVersion(config.LoaderVersion)
	if err != nil {
		return config, err
	}
	if mc != config.Version {
		return config, fmt.Errorf("NeoForge %s 不支持 Minecraft %s，保留固定版本且拒绝安装", config.LoaderVersion, config.Version)
	}
	return config, nil
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

	var matchedVersions []string
	for _, v := range result.Versions {
		mc, err := neoForgeMinecraftVersion(v)
		if err == nil && mc == mcVersion {
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
