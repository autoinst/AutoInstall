package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// InstConfig 结构体表示 inst.json 的内容
type InstConfig struct {
	Version       string `json:"version"`
	Loader        string `json:"loader"`
	LoaderVersion string `json:"loaderVersion"`
	Download      string `json:"download"`
}

// 下载文件的函数
func downloadFile(url, filePath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("无法下载文件: %v", err)
	}
	defer resp.Body.Close()

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("无法创建文件: %v", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("无法写入文件: %v", err)
	}

	return nil
}

func findJava() (string, bool) {
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

	if runtime.GOOS == "linux" {
		fallbackPath := "/usr/bin/jdk/jdk1.8.0_361/bin/java"
		if _, err := os.Stat(fallbackPath); err == nil {
			return fallbackPath, true
		}
	}

	return "", false
}

func main() {
	instFile := "inst.json"
	var config InstConfig
	if _, err := os.Stat(instFile); err == nil {
		data, err := os.ReadFile(instFile)
		if err != nil {
			log.Println("无法读取 inst.json 文件:", err)
			return
		}

		if err := json.Unmarshal(data, &config); err != nil {
			log.Println("无法解析 inst.json 文件:", err)
			return
		}

		fmt.Println("准备安装:")
		fmt.Printf("Minecraft版本: %s\n", config.Version)
		fmt.Printf("加载器: %s\n", config.Loader)
		fmt.Printf("加载器版本: %s\n", config.LoaderVersion)
		fmt.Printf("下载源: %s\n", config.Download)

		javaPath, simpfun := findJava()
		if javaPath == "" {
			log.Println("未找到 Java，请确保已安装 Java 并设置 JAVA_HOME。")
			return
		}
		fmt.Println("找到 Java 运行环境:", javaPath)
		if simpfun {
			fmt.Println("使用备用 Java 位置，已启用simpfun特调")
		}

		if config.Loader == "neoforge" && config.Download == "bmclapi" {
			installerURL := fmt.Sprintf(
				"https://bmclapi2.bangbang93.com/maven/net/neoforged/neoforge/%s/neoforge-%s-installer.jar",
				config.LoaderVersion, config.LoaderVersion,
			)
			installerPath := filepath.Join("./.autoinst/cache", fmt.Sprintf("neoforge-%s-installer.jar", config.LoaderVersion))
			fmt.Println("检测到 neoforge 加载器，正在从 BMCLAPI 下载:", installerURL)
			if err := downloadFile(installerURL, installerPath); err != nil {
				log.Println("下载 neoforge 失败:", err)
			} else {
				fmt.Println("neoforge 安装器下载完成:", installerPath)
			}
		}
	} else if os.IsNotExist(err) {
		log.Println("inst.json 文件不存在")
	} else {
		log.Println("无法访问 inst.json 文件:", err)
	}
}
