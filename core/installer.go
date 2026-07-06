package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

func installerJava(simpfun bool, mise bool) string {
	if simpfun {
		if mise {
			_ = exec.Command("mise", "use", "-g", "java@zulu-8.86.0.25").Run()
			return "java"
		}
		return "/usr/bin/jdk/jdk1.8.0_361/bin/java"
	}
	return "java"
}

func runtimeJava(mc string) int {
	mc = strings.Split(mc, "-")[0]
	lowerMC := strings.ToLower(mc)

	if strings.Contains(lowerMC, "snapshot") {
		return 25
	}

	if match := regexp.MustCompile(`^(\d{2})w(\d{2})([a-z]?)$`).FindStringSubmatch(lowerMC); len(match) == 4 {
		week, _ := strconv.Atoi(match[1])
		if week >= 26 {
			return 25
		}
		return 21
	}

	if strings.Contains(lowerMC, "w") {
		return 21
	}

	parts := strings.Split(mc, ".")
	if len(parts) >= 2 && parts[0] == "1" {
		minor, _ := strconv.Atoi(parts[1])
		patch := 0
		if len(parts) >= 3 {
			patch, _ = strconv.Atoi(parts[2])
		}

		switch minor {
		case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16:
			return 8
		case 17:
			return 16
		case 18, 19:
			return 17
		case 20:
			if patch <= 4 {
				return 17
			}
			return 21
		case 21:
			if patch <= 11 {
				return 21
			}
			return 25
		default:
			return 25
		}
	}

	if major, err := strconv.Atoi(parts[0]); err == nil {
		if major >= 26 {
			return 25
		}
	}

	return 21
}

func RunInstaller(
	installerPath string,
	loader string,
	version string,
	loaderVersion string,
	Download string,
	simpfun bool,
	mise bool,
) error {

	javaPath := installerJava(simpfun, mise)

	var cmd *exec.Cmd

	if Download == "bmclapi" {
		switch loader {
		case "forge", "neoforge":
			cmd = exec.Command(
				javaPath, "-jar", installerPath,
				"--installServer",
				"--mirror", "https://bmclapi2.bangbang93.com/maven/",
			)
		case "fabric":
			cmd = exec.Command(
				javaPath, "-jar", installerPath, "server",
				"-mcversion", version,
				"-loader", loaderVersion,
				"-mavenurl", "https://bmclapi2.bangbang93.com/maven/",
				"-metaurl", "https://bmclapi2.bangbang93.com/fabric-meta/",
			)
		default:
			cmd = exec.Command(javaPath, "-jar", installerPath)
		}
	} else {
		switch loader {
		case "forge", "neoforge":
			cmd = exec.Command(javaPath, "-jar", installerPath, "--installServer")
		case "fabric":
			cmd = exec.Command(
				javaPath, "-jar", installerPath, "server",
				"-mcversion", version,
				"-loader", loaderVersion,
			)
		default:
			cmd = exec.Command(javaPath, "-jar", installerPath)
		}
	}

	stdoutWriter := newStreamLogWriter("installer.stdout", os.Stdout)
	stderrWriter := newStreamLogWriter("installer.stderr", os.Stderr)
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	Logf("运行安装器命令: %s\n", strings.Join(cmd.Args, " "))

	if err := cmd.Start(); err != nil {
		return err
	}
	err := cmd.Wait()
	stdoutWriter.Flush()
	stderrWriter.Flush()
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
) error {
	retries = NormalizeRetries(retries)
	currentSource := downloadSource
	if currentSource == "" {
		currentSource = "official"
	}

	fallbackSource := "bmclapi"
	if currentSource == "bmclapi" {
		fallbackSource = "official"
	}

	currentErr := runInstallerRetry(installerPath, loader, version, loaderVersion, currentSource, simpfun, mise, retries, installFilePath)
	if currentErr == nil {
		return nil
	}

	Log("当前源运行安装器失败，切换备用源")
	fallbackErr := runInstallerRetry(installerPath, loader, version, loaderVersion, fallbackSource, simpfun, mise, retries, installFilePath)
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
) error {
	var lastErr error
	for i := 0; i < retries; i++ {
		if err := RunInstaller(installerPath, loader, version, loaderVersion, downloadSource, simpfun, mise); err != nil {
			lastErr = err
			Logf("运行安装器失败 %d/%d: %v\n", i+1, retries, err)
			continue
		}
		// if err := ValidateInstalledServerArtifacts(version, loader, loaderVersion, installFilePath); err != nil {
		// 	lastErr = fmt.Errorf("安装器退出成功但产物校验失败: %w", err)
		// 	Logf("运行安装器失败 %d/%d: %v\n", i+1, retries, lastErr)
		// 	continue
		// }
		return nil
	}
	return fmt.Errorf("多次运行安装器失败 (共 %d 次): %w", retries, lastErr)
}

func FindJava() (string, bool, bool) {
	simpfun := false
	mise := false

	if runtime.GOOS == "linux" {
		if _, err := os.Stat("/home/container/.aio"); err == nil {
			simpfun = true
		}
		if _, err := os.Stat("/usr/bin/jdk/jdk1.8.0_361/bin/java"); err == nil {
			simpfun = true
		}
	}

	if simpfun {
		if err := exec.Command("mise", "-v").Run(); err == nil {
			mise = true
		}
	}

	if javaHome := os.Getenv("JAVA_HOME"); javaHome != "" {
		java := filepath.Join(javaHome, "bin", "java")
		if _, err := os.Stat(java); err == nil {
			return java, simpfun, mise
		}
	}

	if err := exec.Command("java", "-version").Run(); err == nil {
		return "java", simpfun, mise
	}

	return "", simpfun, mise
}

func ValidateInstalledServerArtifacts(version, loader, loaderVersion, installFilePath string) error {
	if loader != "forge" && loader != "neoforge" {
		return nil
	}

	artifactPath, err := installedServerArtifactPath(version, loader, loaderVersion, installFilePath)
	if err != nil {
		return err
	}
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

func RunScript(
	Version string,
	Loader string,
	LoaderVersion string,
	simpfun bool,
	mise bool,
	argsment string,
	installFilePath string,
) error {
	// if err := ValidateInstalledServerArtifacts(Version, Loader, LoaderVersion, installFilePath); err != nil {
	// 	return err
	// }

	mem, err := strconv.Atoi(os.Getenv("SERVER_MEMORY"))
	if err != nil || mem <= 1500 {
		mem = 4096 + 1500
	}
	maxmem := mem - 1500
	argsment = strings.ReplaceAll(argsment, "{maxmen}", strconv.Itoa(maxmem))

	javaMajor := runtimeJava(Version)
	var javaPath string

	if simpfun {
		switch javaMajor {
		case 8:
			javaPath = "/usr/bin/jdk/jdk1.8.0_361/bin/java"
		case 16:
			javaPath = "/usr/bin/jdk/jdk-16/bin/java"
		case 17:
			javaPath = "/usr/bin/jdk/jdk-17.0.6/bin/java"
		case 21:
			javaPath = "/usr/bin/jdk/jdk-21.0.2/bin/java"
		default:
			javaPath = "/usr/bin/jdk/jdk-25.0.2/bin/java"
		}
	} else {
		javaPath = "java"
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
				"%s %s -jar %s",
				javaPath, argsment, filepath.ToSlash(artifactPath),
			)
		} else {
			script = fmt.Sprintf(
				"%s %s @libraries/net/minecraftforge/forge/%s-%s/unix_args.txt \"$@\"",
				javaPath, argsment, Version, LoaderVersion,
			)
		}

	case "neoforge":
		script = fmt.Sprintf(
			"%s %s @libraries/net/neoforged/neoforge/%s/unix_args.txt \"$@\"",
			javaPath, argsment, LoaderVersion,
		)

	case "fabric":
		script = fmt.Sprintf(
			"%s %s -jar fabric-server-launch.jar",
			javaPath, argsment,
		)
	case "vanilla":
		script = fmt.Sprintf(
			"%s %s -jar server.jar",
			javaPath, argsment,
		)
	}

	if script == "" {
		return fmt.Errorf("无法为加载器 %s 生成启动脚本", Loader)
	}
	script = "#!/bin/bash\n" + script + "\n"
	tmp := "run.sh.tmp"
	if err := os.WriteFile(tmp, []byte(script), 0777); err != nil {
		return err
	}
	if err := os.Rename(tmp, "run.sh"); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
