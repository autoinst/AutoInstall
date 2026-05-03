package pkg

import (
	"fmt"
	"os"
	"regexp"

	"github.com/autoinst/AutoInstall/core"
)

func Common(config core.InstConfig, cleaninst bool) error {
	core.ApplyConfigDefaults(&config)

	javaPath, simpfun, mise := core.FindJava()
	if simpfun {
		core.Log("已启用 simpfun 环境")
		if mise {
			core.Log("启用mise")
		}
	} else {
		if javaPath == "" {
			return fmt.Errorf("未找到 Java，请确保已安装 Java 并设置 PATH")
		}
		core.Log("找到 Java 运行环境:", javaPath)
	}

	if config.Version != "latest" {
		matched, _ := regexp.MatchString(`[a-zA-Z]`, config.Version)
		if matched && (config.Loader == "neoforge" || config.Loader == "forge") {
			return fmt.Errorf("安装器不支持安装(Neo)Forge快照/愚人节版本，请使用原版或 Fabric 加载器")
		}
	}

	switch config.Loader {
	case "neoforge":
		if err := NeoForgeB(config, simpfun, mise); err != nil {
			return err
		}
	case "forge":
		if err := ForgeB(config, simpfun, mise); err != nil {
			return err
		}
	case "fabric":
		if err := FabricB(config, simpfun, mise); err != nil {
			return err
		}
	case "vanilla":
		if config.Version == "latest" {
			latestSnapshot, err := FetchLatestVanillaVersion(config.Download, config.MaxRetries)
			if err != nil {
				return fmt.Errorf("获取最新 Minecraft 版本失败: %w", err)
			}
			config.Version = latestSnapshot
		}

		librariesDir := "./libraries"
		if err := DownloadServerJar(config.Version, config.Loader, librariesDir, config.Download, config.MaxRetries); err != nil {
			return fmt.Errorf("下载 mc 服务端失败: %w", err)
		}
		core.Log("服务端下载完成")
		if err := core.RunScript(config.Version, config.Loader, config.LoaderVersion, simpfun, mise, config.Argsment); err != nil {
			return fmt.Errorf("生成启动脚本失败: %w", err)
		}
	default:
		return fmt.Errorf("未知加载器: %s", config.Loader)
	}

	if cleaninst {
		core.Log("正在清理残留...")
		_ = os.Remove("modrinth.index.json")
		if err := os.Remove(".autoinst"); err != nil && !os.IsNotExist(err) {
			core.Log("删除 .autoinst 文件失败:", err)
		}
		files, err := os.ReadDir(".")
		if err == nil {
			for _, file := range files {
				if !file.IsDir() && len(file.Name()) > 4 && file.Name()[len(file.Name())-4:] == ".log" {
					if err := os.Remove(file.Name()); err != nil {
						core.Log("删除日志文件失败:", file.Name(), err)
					}
				}
			}
		} else {
			core.Log("读取目录失败:", err)
		}
		core.Log("清理完成")
	}
	return nil
}

func FetchLatestVanillaVersion(downloadSource string, maxRetries int) (string, error) {
	officialURL := "https://launchermeta.mojang.com/mc/game/version_manifest.json"
	mirrorURL := "https://bmclapi2.bangbang93.com/mc/game/version_manifest.json"
	currentURL, fallbackURL := sourceURLPair(officialURL, mirrorURL, downloadSource)

	var result struct {
		Latest struct {
			Snapshot string `json:"snapshot"`
		} `json:"latest"`
	}
	if err := getJSONWithFallback(currentURL, fallbackURL, maxRetries, &result); err != nil {
		return "", err
	}
	return result.Latest.Snapshot, nil
}

func fetchVersionManifest(downloadSource string, maxRetries int) (struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
}, error) {
	officialURL := "https://launchermeta.mojang.com/mc/game/version_manifest.json"
	mirrorURL := "https://bmclapi2.bangbang93.com/mc/game/version_manifest.json"
	currentURL, fallbackURL := sourceURLPair(officialURL, mirrorURL, downloadSource)

	var result struct {
		Latest struct {
			Release  string `json:"release"`
			Snapshot string `json:"snapshot"`
		} `json:"latest"`
	}
	if err := getJSONWithFallback(currentURL, fallbackURL, maxRetries, &result); err != nil {
		return result, err
	}
	return result, nil
}

func sourceURLPair(officialURL, mirrorURL, downloadSource string) (string, string) {
	if downloadSource == "bmclapi" {
		return mirrorURL, officialURL
	}
	return officialURL, mirrorURL
}
