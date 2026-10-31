package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	runtimeenv "github.com/autoinst/AutoInstall/internal/runtime"
	"strconv"
	"strings"
)

// installerCommandRunner is a command execution seam; tests never launch Java.
var installerCommandRunner = func(cmd *exec.Cmd) error { return cmd.Run() }

func installerContext(contexts []context.Context) context.Context {
	for _, ctx := range contexts {
		if ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

func javaHomeExecutable() string { return runtimeenv.JavaHomeExecutable() }

func installerJava(simpfun bool, mise bool, versions ...string) string {
	return runtimeenv.InstallerJava(simpfun, mise, versions...)
}

func runtimeJava(mc string) int { return runtimeenv.JavaMajor(mc) }

func RunInstaller(
	installerPath string,
	loader string,
	version string,
	loaderVersion string,
	Download string,
	simpfun bool,
	mise bool,
	contexts ...context.Context,
) error {
	ctx := installerContext(contexts)
	if err := ctx.Err(); err != nil {
		return err
	}
	javaPath := installerJava(simpfun, mise, version)

	var cmd *exec.Cmd

	if Download == "bmclapi" {
		switch loader {
		case "forge", "neoforge":
			cmd = exec.CommandContext(
				ctx, javaPath, "-jar", installerPath,
				"--installServer",
				"--mirror", "https://bmclapi2.bangbang93.com/maven/",
			)
		case "fabric":
			cmd = exec.CommandContext(
				ctx, javaPath, "-jar", installerPath, "server",
				"-mcversion", version,
				"-loader", loaderVersion,
				"-mavenurl", "https://bmclapi2.bangbang93.com/maven/",
				"-metaurl", "https://bmclapi2.bangbang93.com/fabric-meta/",
			)
		default:
			cmd = exec.CommandContext(ctx, javaPath, "-jar", installerPath)
		}
	} else {
		switch loader {
		case "forge", "neoforge":
			cmd = exec.CommandContext(ctx, javaPath, "-jar", installerPath, "--installServer")
		case "fabric":
			cmd = exec.CommandContext(
				ctx, javaPath, "-jar", installerPath, "server",
				"-mcversion", version,
				"-loader", loaderVersion,
			)
		default:
			cmd = exec.CommandContext(ctx, javaPath, "-jar", installerPath)
		}
	}

	if simpfun && mise {
		// Scope the selected installer JDK to this process, never mise use -g.
		cmd = exec.CommandContext(ctx, "mise", append([]string{"exec", fmt.Sprintf("java@%d", runtimeJava(version)), "--"}, cmd.Args...)...)
	}

	stdoutWriter := newStreamLogWriter("installer.stdout", os.Stdout)
	stderrWriter := newStreamLogWriter("installer.stderr", os.Stderr)
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	Logf("运行安装器命令: %s\n", strings.Join(cmd.Args, " "))

	defer stdoutWriter.Flush()
	defer stderrWriter.Flush()
	err := installerCommandRunner(cmd)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func RunInstallerWithFallback(
	installerPath string,
	loader string,
	version string,
	loaderVersion string,
	downloadSource string,
	simpfun bool,
	mise bool,
	retries int,
	installFilePath string,
	contexts ...context.Context,
) error {
	ctx := installerContext(contexts)
	if err := ctx.Err(); err != nil {
		return err
	}
	retries = NormalizeRetries(retries)
	currentSource := downloadSource
	if currentSource == "" {
		currentSource = "official"
	}

	fallbackSource := "bmclapi"
	if currentSource == "bmclapi" {
		fallbackSource = "official"
	}

	currentErr := runInstallerRetry(installerPath, loader, version, loaderVersion, currentSource, simpfun, mise, retries, installFilePath, ctx)
	if currentErr == nil {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	Log("当前源运行安装器失败，切换备用源")
	fallbackErr := runInstallerRetry(installerPath, loader, version, loaderVersion, fallbackSource, simpfun, mise, retries, installFilePath, ctx)
	if fallbackErr == nil {
		return nil
	}
	return fmt.Errorf("当前源运行安装器失败: %w; 备用源运行安装器失败: %w", currentErr, fallbackErr)
}

func runInstallerRetry(
	installerPath string,
	loader string,
	version string,
	loaderVersion string,
	downloadSource string,
	simpfun bool,
	mise bool,
	retries int,
	installFilePath string,
	contexts ...context.Context,
) error {
	ctx := installerContext(contexts)
	if err := ctx.Err(); err != nil {
		return err
	}
	retries = NormalizeRetries(retries)
	var lastErr error
	for i := 0; i < retries; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := RunInstaller(installerPath, loader, version, loaderVersion, downloadSource, simpfun, mise, ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			Logf("运行安装器失败 %d/%d: %v\n", i+1, retries, err)
			continue
		}
		if err := validateInstallerArtifacts(installerPath, version, loader, loaderVersion, installFilePath); err != nil {
			lastErr = fmt.Errorf("安装器退出成功但产物校验失败: %w", err)
			Logf("运行安装器失败 %d/%d: %v\n", i+1, retries, lastErr)
			continue
		}
		return ctx.Err()
	}
	return fmt.Errorf("多次运行安装器失败 (共 %d 次): %w", retries, lastErr)
}

func validateInstallerArtifacts(installerPath, version, loader, loaderVersion, installFilePath string) error {
	if err := ValidateInstalledServerArtifacts(version, loader, loaderVersion, installFilePath); err != nil {
		return err
	}
	if loader == "forge" || loader == "neoforge" {
		info, err := ExtractVersionJson(installerPath)
		if err != nil {
			return fmt.Errorf("读取安装器产物元数据失败: %w", err)
		}
		if err := VerifyRequiredServerArtifacts(".", info); err != nil {
			return err
		}
	}
	return nil
}

func FindJava() (string, bool, bool) { return runtimeenv.FindJava(context.Background()) }

func FindJavaContext(ctx context.Context) (string, bool, bool) { return runtimeenv.FindJava(ctx) }

func ValidateInstalledServerArtifacts(version, loader, loaderVersion, installFilePath string) error {
	var artifactPath string
	var err error
	switch loader {
	case "forge", "neoforge":
		artifactPath, err = installedServerArtifactPath(version, loader, loaderVersion, installFilePath)
	case "fabric":
		if err := validateNonemptyArtifact("server.jar"); err != nil {
			return err
		}
		artifactPath = "fabric-server-launch.jar"
	case "vanilla":
		artifactPath = "server.jar"
	default:
		return fmt.Errorf("无法为加载器 %s 校验安装产物", loader)
	}
	if err != nil {
		return err
	}
	return validateNonemptyArtifact(artifactPath)
}

func validateNonemptyArtifact(artifactPath string) error {
	info, err := os.Stat(artifactPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("安装产物不存在: %s", artifactPath)
		}
		return fmt.Errorf("检查安装产物失败 %s: %w", artifactPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("安装产物不是文件: %s", artifactPath)
	}
	if info.Size() <= 0 {
		return fmt.Errorf("安装产物为空: %s", artifactPath)
	}
	return nil
}

func installedServerArtifactPath(version, loader, loaderVersion, installFilePath string) (string, error) {
	switch loader {
	case "forge":
		if runtimeJava(version) <= 8 {
			if path := strings.TrimSpace(installFilePath); path != "" {
				cleanPath, err := cleanRelativeArtifactPath(path)
				if err != nil {
					return "", err
				}
				return cleanPath, nil
			}
			return fmt.Sprintf("forge-%s-%s.jar", version, loaderVersion), nil
		}
		return filepath.Join("libraries", "net", "minecraftforge", "forge", version+"-"+loaderVersion, "unix_args.txt"), nil
	case "neoforge":
		return filepath.Join("libraries", "net", "neoforged", "neoforge", loaderVersion, "unix_args.txt"), nil
	default:
		return "", fmt.Errorf("无法为加载器 %s 校验安装产物", loader)
	}
}

func cleanRelativeArtifactPath(path string) (string, error) {
	path = filepath.Clean(filepath.FromSlash(path))
	if path == "." || filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("安装产物路径不合法: %s", path)
	}
	return path, nil
}

// RunScript validates the loader's launch artifacts before replacing run.sh.
// Forge/NeoForge callers should also pass their extracted metadata so processor
// outputs and available hashes are checked, not only the launch args file.
func RunScript(
	Version string,
	Loader string,
	LoaderVersion string,
	simpfun bool,
	mise bool,
	argsment string,
	installFilePath string,
	metadata ...VersionInfo,
) error {
	if err := ValidateInstalledServerArtifacts(Version, Loader, LoaderVersion, installFilePath); err != nil {
		return err
	}
	for _, info := range metadata {
		if err := VerifyRequiredServerArtifacts(".", info); err != nil {
			return err
		}
	}

	mem, err := strconv.Atoi(os.Getenv("SERVER_MEMORY"))
	if err != nil || mem <= 1500 {
		mem = 4096 + 1500
	}
	maxmem := mem - 1500
	argsment = strings.ReplaceAll(argsment, "{maxmen}", strconv.Itoa(maxmem))

	javaMajor := runtimeJava(Version)
	javaPath := installerJava(simpfun, false, Version)
	javaCommand := shellQuote(filepath.ToSlash(javaPath))
	if simpfun && mise {
		javaCommand = fmt.Sprintf("mise exec java@%d -- java", javaMajor)
	}

	var script string

	switch Loader {
	case "forge":
		if javaMajor <= 8 {
			artifactPath, err := installedServerArtifactPath(Version, Loader, LoaderVersion, installFilePath)
			if err != nil {
				return err
			}
			script = fmt.Sprintf(
				"%s %s -jar %s \"$@\"",
				javaCommand, argsment, shellQuote(filepath.ToSlash(artifactPath)),
			)
		} else {
			script = fmt.Sprintf(
				"%s %s %s \"$@\"",
				javaCommand, argsment, shellQuote("@libraries/net/minecraftforge/forge/"+Version+"-"+LoaderVersion+"/unix_args.txt"),
			)
		}

	case "neoforge":
		script = fmt.Sprintf(
			"%s %s %s \"$@\"",
			javaCommand, argsment, shellQuote("@libraries/net/neoforged/neoforge/"+LoaderVersion+"/unix_args.txt"),
		)

	case "fabric":
		script = fmt.Sprintf(
			"%s %s -jar fabric-server-launch.jar \"$@\"",
			javaCommand, argsment,
		)
	case "vanilla":
		script = fmt.Sprintf(
			"%s %s -jar server.jar \"$@\"",
			javaCommand, argsment,
		)
	}

	if script == "" {
		return fmt.Errorf("无法为加载器 %s 生成启动脚本", Loader)
	}
	script = "#!/bin/bash\n" + script + "\n"
	f, err := os.CreateTemp(".", ".autoinst-run-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.WriteString(script); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0755); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Recheck before replacing our script. An official installer may already
	// have written its own run.sh; failed validation never overwrites it here.
	if err := ValidateInstalledServerArtifacts(Version, Loader, LoaderVersion, installFilePath); err != nil {
		return err
	}
	for _, info := range metadata {
		if err := VerifyRequiredServerArtifacts(".", info); err != nil {
			return err
		}
	}
	return os.Rename(tmp, "run.sh")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
