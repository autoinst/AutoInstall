package core

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func getJavaMajor(version string) int {
	v := strings.ToLower(version)

	if len(v) >= 3 && v[2] == 'w' {
		year, err := strconv.Atoi(v[:2])
		if err == nil {
			return year + 1
		}
	}

	main := strings.Split(v, "-")[0]
	parts := strings.Split(main, ".")
	if len(parts) == 0 {
		return 0
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	return major
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

	javaMajor := getJavaMajor(version)
	var javaPath string

	if simpfun {
		if javaMajor >= 26 {
			return fmt.Errorf("不支持 Minecraft %s（需要 Java 25）", version)
		}
		if mise {
			var cmd *exec.Cmd
			switch {
			case javaMajor < 17:
				cmd = exec.Command("mise", "use", "-g", "java@zulu-8.86.0.25")
			case javaMajor < 21:
				cmd = exec.Command("mise", "use", "-g", "java@zulu-17.58.21")
			case javaMajor < 26:
				cmd = exec.Command("mise", "use", "-g", "java@zulu-21.42.19")
			default:
				cmd = exec.Command("mise", "use", "-g", "java@zulu-25")
			}
			_ = cmd.Run()
			javaPath = "java"
		} else {
			javaPath = "/usr/bin/jdk/jdk1.8.0_361/bin/java"
		}
	} else {
		javaPath = "java"
	}
	var cmd *exec.Cmd

	if Download == "bmclapi" {
		switch loader {
		case "forge", "neoforge":
			cmd = exec.Command(javaPath, "-jar", installerPath,
				"--installServer",
				"--mirror", "https://bmclapi2.bangbang93.com/maven/",
			)
		case "fabric":
			cmd = exec.Command(
				javaPath, "-jar", installerPath, "server",
				"-mavenurl", "https://bmclapi2.bangbang93.com/maven/",
				"-metaurl", "https://bmclapi2.bangbang93.com/fabric-meta/",
				"-mcversion", version,
				"-loader", loaderVersion,
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

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return err
	}

	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)

	return cmd.Wait()
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

func RunScript(
	Version string,
	Loader string,
	LoaderVersion string,
	simpfun bool,
	mise bool,
	argsment string,
) {
	_ = os.Remove("run.sh")

	mem, err := strconv.Atoi(os.Getenv("SERVER_MEMORY"))
	if err != nil || mem <= 1500 {
		mem = 4096 + 1500
	}
	maxmem := mem - 1500
	argsment = strings.ReplaceAll(argsment, "{maxmen}", strconv.Itoa(maxmem))
	javaMajor := getJavaMajor(Version)
	var javaPath string

	if simpfun {
		if javaMajor >= 26 {
			Log("不支持 Minecraft", Version, "（需要 Java 25）")
			return
		}

		switch {
		case javaMajor < 17:
			javaPath = "/usr/bin/jdk/jdk1.8.0_361/bin/java"
		case javaMajor < 21:
			javaPath = "/usr/bin/jdk/jdk-17.0.6/bin/java"
		case javaMajor < 26:
			javaPath = "/usr/bin/jdk/jdk-21.0.2/bin/java"
		default:
			javaPath = "/usr/bin/jdk/jdk-25/bin/java"
		}
	} else {
		javaPath = "java"
	}

	var script string

	switch Loader {
	case "forge":
		if javaMajor < 17 {
			script = fmt.Sprintf("%s %s -jar forge-%s-%s.jar",
				javaPath, argsment, Version, LoaderVersion)
		} else {
			script = fmt.Sprintf("%s %s @libraries/net/minecraftforge/forge/%s-%s/unix_args.txt \"$@\"",
				javaPath, argsment, Version, LoaderVersion)
		}
	case "neoforge":
		script = fmt.Sprintf("%s %s @libraries/net/neoforged/neoforge/%s/unix_args.txt \"$@\"",
			javaPath, argsment, LoaderVersion)
	case "fabric":
		script = fmt.Sprintf("%s %s -jar fabric-server-launch.jar",
			javaPath, argsment)
	}

	_ = os.WriteFile("run.sh", []byte(script), 0777)
}
