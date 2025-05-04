package core

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func RunInstaller(installerPath string, loader string, version string, loaderVersion string, Download string, simpfun bool) error {
	var javaPath string
	if simpfun {
		javaPath = "/usr/bin/jdk/jdk1.8.0_361/bin/java"
	} else {
		javaPath = "java"
	}
	var cmd *exec.Cmd
	if Download == "bmclapi" {
		if loader == "forge" {
			cmd = exec.Command(javaPath, "-jar", installerPath, "--installServer", "--mirror", "https://bmclapi2.bangbang93.com/maven/")
		} else if loader == "neoforge" {
			cmd = exec.Command(javaPath, "-jar", installerPath, "--installServer", "--mirror", "https://bmclapi2.bangbang93.com/maven/")
		} else if loader == "fabric" {
			cmd = exec.Command(
				javaPath, "-jar", installerPath, "server",
				"-mavenurl", "https://bmclapi2.bangbang93.com/maven/",
				"-metaurl", "https://bmclapi2.bangbang93.com/fabric-meta/",
				"-mcversion", version,
				"-loader", loaderVersion,
			)
		} else {
			cmd = exec.Command(javaPath, "-jar", installerPath)
		}
	} else {
		if loader == "forge" {
			cmd = exec.Command(javaPath, "-jar", installerPath, "--installServer")
		} else if loader == "neoforge" {
			cmd = exec.Command(javaPath, "-jar", installerPath, "--installServer")
		} else if loader == "fabric" {
			cmd = exec.Command(
				javaPath, "-jar", installerPath, "server",
				"-mcversion", version,
				"-loader", loaderVersion,
			)
		} else {
			cmd = exec.Command(javaPath, "-jar", installerPath)
		}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("无法获取标准输出: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("无法获取标准错误: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动命令失败: %v", err)
	}
	go func() {
		io.Copy(os.Stdout, stdout)
	}()
	go func() {
		io.Copy(os.Stderr, stderr)
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("命令执行失败: %v", err)
	}
	return nil
}

func FindJava() (string, bool) {
	if runtime.GOOS == "linux" {
		simpfun := "/usr/bin/jdk/jdk1.8.0_361/bin/java"
		if _, err := os.Stat(simpfun); err == nil {
			return simpfun, true
		}
	}

	javaHome := os.Getenv("JAVA_HOME")
	if javaHome != "" {
		javaPath := filepath.Join(javaHome, "bin", "java")
		if _, err := os.Stat(javaPath); err == nil {
			return javaPath, false
		}
	}

	cmd := exec.Command("java", "-version")
	output, err := cmd.CombinedOutput()
	if err == nil && strings.Contains(string(output), "version") {
		return "java", false
	}

	return "", false
}
