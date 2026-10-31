package pkg

import (
	"fmt"

	"github.com/autoinst/AutoInstall/core"
)

// installForgeFamilyServer runs the shared Forge/NeoForge server installation tail.
// Target selection and installer downloads belong to each loader. Fabric's launch-jar
// installation is deliberately separate, and processors stay in the official installer.
func installForgeFamilyServer(config core.InstConfig, installerPath string, simpfun, mise bool) error {
	return runForgeServerInstall(config, forgeServerInstallSteps{
		metadata: func() (core.VersionInfo, error) {
			return core.ExtractVersionJson(installerPath)
		},
		prefetch: func(info core.VersionInfo) error {
			return DownloadLibrariesAndServerJar(info, config.Version, config.Loader, "./libraries", config.MaxConnections, config.Download, config.MaxRetries)
		},
		installer: func(info core.VersionInfo) error {
			return core.RunInstallerWithFallback(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download, simpfun, mise, config.MaxRetries, info.InstallFilePath, config.Context)
		},
		script: func(info core.VersionInfo) error {
			return core.RunScript(config.Version, config.Loader, config.LoaderVersion, simpfun, mise, config.Argsment, info.InstallFilePath, info)
		},
	})
}

// These four operations permit offline orchestration tests without Java or network calls.
// This is not a processor runner: installer is always the official installer in production.
type forgeServerInstallSteps struct {
	metadata  func() (core.VersionInfo, error)
	prefetch  func(core.VersionInfo) error
	installer func(core.VersionInfo) error
	script    func(core.VersionInfo) error
}

func runForgeServerInstall(config core.InstConfig, steps forgeServerInstallSteps) error {
	info, err := steps.metadata()
	if err != nil {
		return fmt.Errorf("提取安装器元数据失败: %w", err)
	}
	if info.MinecraftVersion != "" && info.MinecraftVersion != config.Version {
		return fmt.Errorf("%s 安装器支持 Minecraft %s，与请求 %s 不匹配", config.Loader, info.MinecraftVersion, config.Version)
	}
	if err := steps.prefetch(info); err != nil {
		return err
	}
	core.Log("库文件和服务端下载完成")
	if err := steps.installer(info); err != nil {
		return fmt.Errorf("运行安装器失败: %w", err)
	}
	if err := steps.script(info); err != nil {
		return fmt.Errorf("生成启动脚本失败: %w", err)
	}
	return nil
}
