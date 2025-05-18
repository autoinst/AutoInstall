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

func FindJava() (string, bool, bool) {
	simpfun := false
	mise := false
	if runtime.GOOS == "linux" {
		if _, err := os.Stat("/home/container/.aio"); err == nil {
			simpfun = true
		}
		javaPath := "/usr/bin/jdk/jdk1.8.0_361/bin/java"
		if _, err := os.Stat(javaPath); err == nil {
			simpfun = true
		}
	}

	if simpfun {
		cmd := exec.Command("mise", "-v")
		if err := cmd.Run(); err == nil {
			mise = true
		}
	}

	javaHome := os.Getenv("JAVA_HOME")
	if javaHome != "" {
		javaPath := filepath.Join(javaHome, "bin", "java")
		if _, err := os.Stat(javaPath); err == nil {
			return javaPath, simpfun, mise
		}
	}

	cmd := exec.Command("java", "-version")
	output, err := cmd.CombinedOutput()
	if err == nil && strings.Contains(string(output), "version") {
		return "java", simpfun, mise
	}

	return "", simpfun, mise
}

func RunScript(Version string, Loader string, LoaderVersion string, simpfun bool, mise bool) {
	if _, err := os.Stat("run.sh"); err == nil {
		if err := os.Remove("run.sh"); err != nil {
			fmt.Println("删除旧的 run.sh 失败:", err)
			return
		}
	}
	var scriptContent string
	if Loader == "forge" {
		scriptContent = fmt.Sprintf("java @libraries/net/minecraftforge/forge/%s-%s/unix_args.txt \"$@\"", Version, LoaderVersion)
	} else if Loader == "neoforge" {
		scriptContent = fmt.Sprintf("java @libraries/net/neoforged/neoforge/%s/unix_args.txt \"$@\"", LoaderVersion)
	} else if Loader == "fabric" {
		scriptContent = "java -jar fabric-server-launch.jar"
	}
	file, err := os.Create("run.sh")
	if err != nil {
		fmt.Println("创建文件失败:", err)
		return
	}
	defer file.Close()
	_, err = file.WriteString(scriptContent)
	if err != nil {
		fmt.Println("写入文件失败:", err)
		return
	}
	err = os.Chmod("run.sh", 0777)
	if err != nil {
		fmt.Println("修改权限失败:", err)
		return
	}
}
