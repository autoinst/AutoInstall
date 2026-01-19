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

	if strings.Contains(mc, "w") {
		return 21
	}

	if strings.HasPrefix(mc, "1.") {
		parts := strings.Split(mc, ".")
		if len(parts) >= 2 {
			minor, _ := strconv.Atoi(parts[1])
			switch {
			case minor <= 16:
				return 8
			case minor <= 17:
				return 16
			case minor <= 20:
				return 17
			default:
				return 21
			}
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
		default:
			javaPath = "/usr/bin/jdk/jdk-21.0.2/bin/java"
		}
	} else {
		javaPath = "java"
	}

	var script string

	switch Loader {
	case "forge":
		if javaMajor <= 8 {
			script = fmt.Sprintf(
				"%s %s -jar forge-%s-%s.jar",
				javaPath, argsment, Version, LoaderVersion,
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
	}

	script = "#!/bin/bash\n" + script + "\n"
	_ = os.WriteFile("run.sh", []byte(script), 0777)
}