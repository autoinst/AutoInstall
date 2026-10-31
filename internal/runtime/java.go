package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

func JavaHomeExecutable() string {
	if home := os.Getenv("JAVA_HOME"); home != "" {
		for _, name := range []string{"java", "java.exe"} {
			path := filepath.Join(home, "bin", name)
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				return path
			}
		}
	}
	return ""
}

func InstallerJava(simpfun, mise bool, versions ...string) string {
	if simpfun {
		if mise {
			return "java"
		}
		major := 8
		if len(versions) > 0 {
			major = JavaMajor(versions[0])
		}
		switch major {
		case 8:
			return "/usr/bin/jdk/jdk1.8.0_361/bin/java"
		case 16:
			return "/usr/bin/jdk/jdk-16/bin/java"
		case 17:
			return "/usr/bin/jdk/jdk-17.0.6/bin/java"
		case 21:
			return "/usr/bin/jdk/jdk-21.0.2/bin/java"
		default:
			return "/usr/bin/jdk/jdk-25.0.2/bin/java"
		}
	}
	if java := JavaHomeExecutable(); java != "" {
		return java
	}
	return "java"
}

// JavaMajor retains the installer's existing runtime selection policy.
func JavaMajor(mc string) int {
	mc = strings.Split(mc, "-")[0]
	lower := strings.ToLower(mc)
	if strings.Contains(lower, "snapshot") {
		return 25
	}
	if match := regexp.MustCompile(`^(\d{2})w(\d{2})([a-z]?)$`).FindStringSubmatch(lower); len(match) == 4 {
		year, _ := strconv.Atoi(match[1])
		if year >= 26 {
			return 25
		}
		return 21
	}
	if strings.Contains(lower, "w") {
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
	if major, err := strconv.Atoi(parts[0]); err == nil && major >= 26 {
		return 25
	}
	return 21
}

func FindJava(ctx context.Context) (string, bool, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	simpfun, mise := false, false
	if runtime.GOOS == "linux" {
		for _, path := range []string{"/home/container/.aio", "/usr/bin/jdk/jdk1.8.0_361/bin/java"} {
			if _, err := os.Stat(path); err == nil {
				simpfun = true
			}
		}
	}
	if simpfun {
		mise = exec.CommandContext(ctx, "mise", "-v").Run() == nil
	}
	if java := JavaHomeExecutable(); java != "" {
		return java, simpfun, mise
	}
	if exec.CommandContext(ctx, "java", "-version").Run() == nil {
		return "java", simpfun, mise
	}
	return "", simpfun, mise
}
