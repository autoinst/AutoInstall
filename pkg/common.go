package pkg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/autoinst/AutoInstall/core"
	"github.com/autoinst/AutoInstall/internal/model"
	"github.com/autoinst/AutoInstall/internal/workspace"
)

type loaderInstaller func(core.InstConfig, bool, bool) error

// New loaders register here; their version and installation rules stay local.
var loaderInstallers = map[string]loaderInstaller{
	"forge": ForgeB, "neoforge": NeoForgeB, "fabric": FabricB, "vanilla": installVanilla,
}

func Install(ctx context.Context, req model.InstallRequest, clean bool) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := filepath.Abs(req.Root)
	if err != nil {
		return err
	}
	if filepath.Clean(root) != filepath.Clean(wd) {
		return fmt.Errorf("加载器安装目录必须是当前工作目录；请在目标目录运行 AutoInstall")
	}
	for _, rel := range []string{"libraries", "server.jar", "fabric-server-launch.jar", "run.sh"} {
		if _, err := workspace.SafeJoin(root, rel); err != nil {
			return err
		}
	}
	cfg := core.InstConfig{Context: ctx, Version: req.Version, Loader: req.Loader, LoaderVersion: req.LoaderVersion, Download: req.Download, MaxConnections: req.MaxConnections, MaxRetries: req.MaxRetries, Argsment: req.Argsment}
	return Common(cfg, clean)
}

func Common(cfg core.InstConfig, clean bool) error {
	if cfg.Context != nil {
		if err := cfg.Context.Err(); err != nil {
			return err
		}
	}
	core.ApplyConfigDefaults(&cfg)
	var before map[string]bool
	if clean {
		var err error
		before, err = cacheFiles()
		if err != nil {
			return err
		}
	}
	java, simpfun, mise := core.FindJavaContext(cfg.Context)
	if cfg.Context != nil && cfg.Context.Err() != nil {
		return cfg.Context.Err()
	}
	if simpfun {
		core.Log("已启用 simpfun 环境")
	} else if java == "" {
		return fmt.Errorf("未找到 Java，请确保已安装 Java 并设置 PATH")
	} else {
		core.Log("找到 Java 运行环境:", java)
	}
	if cfg.Version != "latest" {
		matched, _ := regexp.MatchString(`[a-zA-Z]`, cfg.Version)
		if matched && (cfg.Loader == "forge" || cfg.Loader == "neoforge") {
			return fmt.Errorf("安装器不支持 (Neo)Forge 快照/愚人节版本，请使用原版或 Fabric")
		}
	}
	install, ok := loaderInstallers[cfg.Loader]
	if !ok {
		return fmt.Errorf("未知加载器: %s", cfg.Loader)
	}
	if err := install(cfg, simpfun, mise); err != nil {
		return err
	}
	if clean {
		after, err := cacheFiles()
		if err != nil {
			return err
		}
		var errs []error
		for path := range after {
			if !before[path] {
				if err = os.Remove(path); err != nil {
					errs = append(errs, err)
				}
			}
		}
		if err = errors.Join(errs...); err != nil {
			return fmt.Errorf("清理本次缓存失败: %w", err)
		}
		core.Log("已清理本次新增缓存，保留用户输入、既有缓存及日志")
	}
	return nil
}

func cacheFiles() (map[string]bool, error) {
	result := make(map[string]bool)
	root, err := workspace.SafeJoin(".", ".autoinst/cache")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			path, err := workspace.SafeJoin(root, entry.Name())
			if err != nil {
				return nil, err
			}
			result[path] = true
		}
	}
	return result, nil
}

func installVanilla(cfg core.InstConfig, simpfun, mise bool) error {
	if cfg.Version == "latest" {
		v, err := FetchLatestVanillaVersion(cfg.Download, cfg.MaxRetries)
		if err != nil {
			return err
		}
		cfg.Version = v
	}
	if err := DownloadServerJar(cfg.Version, cfg.Loader, "./libraries", cfg.Download, cfg.MaxRetries, ""); err != nil {
		return err
	}
	return core.RunScript(cfg.Version, cfg.Loader, cfg.LoaderVersion, simpfun, mise, cfg.Argsment, "")
}

func FetchLatestVanillaVersion(source string, retries int) (string, error) {
	manifest, err := fetchVersionManifest(source, retries)
	if err != nil {
		return "", err
	}
	if manifest.Latest.Snapshot == "" {
		return "", fmt.Errorf("版本清单缺少 latest.snapshot")
	}
	return manifest.Latest.Snapshot, nil
}

func fetchVersionManifest(source string, retries int) (struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
}, error) {
	var result struct {
		Latest struct {
			Release  string `json:"release"`
			Snapshot string `json:"snapshot"`
		} `json:"latest"`
	}
	current, fallback := sourceURLPair("https://launchermeta.mojang.com/mc/game/version_manifest.json", "https://bmclapi2.bangbang93.com/mc/game/version_manifest.json", source)
	err := getJSONWithFallback(current, fallback, retries, &result)
	return result, err
}

func sourceURLPair(official, mirror, source string) (string, string) {
	if source == "bmclapi" {
		return mirror, official
	}
	return official, mirror
}
