package main

import (
	"archive/zip"
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

// Library 定义库的结构
type Library struct {
	Name      string `json:"name"`
	Downloads struct {
		Artifact struct {
			URL  string `json:"url"`
			Path string `json:"path"`
		} `json:"artifact"`
	} `json:"downloads"`
}

// VersionInfo 定义 version.json 文件的结构
type VersionInfo struct {
	Libraries []Library `json:"libraries"`
}

// 下载 Minecraft server JAR 文件
func downloadServerJar(version, librariesDir string) error {
	downloadURL := fmt.Sprintf("https://bmclapi2.bangbang93.com/version/%s/server", version)
	serverPath := filepath.Join(librariesDir, "net", "minecraft", "server", version, fmt.Sprintf("server-%s.jar", version))

	if err := os.MkdirAll(filepath.Dir(serverPath), os.ModePerm); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	if err := downloadFile(downloadURL, serverPath); err != nil {
		return fmt.Errorf("无法下载服务端文件 %s: %v", serverPath, err)
	}
	fmt.Println("下载完成 Minecraft 服务端:", serverPath)
	return nil
}

func runInstaller(installerPath string) error {
	cmd := exec.Command("java", "-jar", installerPath)

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

// 从 JAR 文件中提取 version.json
func extractVersionJson(jarFilePath string) (VersionInfo, error) {
	var versionInfo VersionInfo
	r, err := zip.OpenReader(jarFilePath)
	if err != nil {
		return versionInfo, fmt.Errorf("无法打开 JAR 文件: %v", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name == "version.json" {
			rc, err := f.Open()
			if err != nil {
				return versionInfo, fmt.Errorf("无法打开 version.json 文件: %v", err)
			}
			defer rc.Close()

			if err := json.NewDecoder(rc).Decode(&versionInfo); err != nil {
				return versionInfo, fmt.Errorf("无法解析 version.json: %v", err)
			}

			return versionInfo, nil
		}
	}
	for _, f := range r.File {
		if f.Name == "install_profile.json" {
			rc, err := f.Open()
			if err != nil {
				return versionInfo, fmt.Errorf("无法打开 install_profile.json 文件: %v", err)
			}
			defer rc.Close()

			if err := json.NewDecoder(rc).Decode(&versionInfo); err != nil {
				return versionInfo, fmt.Errorf("无法解析 install_profile.json: %v", err)
			}

			return versionInfo, nil
		}
	}

	return versionInfo, fmt.Errorf("没有找到文件")
}

// 替换 URL 并下载库
func downloadLibraries(versionInfo VersionInfo, librariesDir string) error {
	if err := os.MkdirAll(librariesDir, os.ModePerm); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	for _, lib := range versionInfo.Libraries {
		url := lib.Downloads.Artifact.URL

		// 替换 URL
		url = strings.Replace(url, "https://files.minecraftforge.net/maven/", "https://bmclapi2.bangbang93.com/maven/", 1)
		url = strings.Replace(url, "https://maven.fabricmc.net/", "https://bmclapi2.bangbang93.com/maven/", 1)
		url = strings.Replace(url, "https://maven.neoforged.net/releases/", "https://bmclapi2.bangbang93.com/maven/", 1)
		url = strings.Replace(url, "https://libraries.minecraft.net/", "https://bmclapi2.bangbang93.com/maven/", 1)

		filePath := filepath.Join(librariesDir, lib.Downloads.Artifact.Path)
		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return fmt.Errorf("无法创建目录: %v", err)
		}

		if err := downloadFile(url, filePath); err != nil {
			return fmt.Errorf("无法下载库文件 %s: %v", lib.Name, err)
		}
		fmt.Println("下载完成:", filePath)
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
	os.MkdirAll(".autoinst/cache", os.ModePerm)
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
			fmt.Println("已启用 simpfun 特调")
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
				return
			}
			fmt.Println("neoforge 安装器下载完成:", installerPath)

			// 提取 version.json
			versionInfo, err := extractVersionJson(installerPath)
			if err != nil {
				log.Println("提取 version.json 失败:", err)
				return
			}

			librariesDir := "./libraries"
			if err := downloadLibraries(versionInfo, librariesDir); err != nil {
				log.Println("下载库文件失败:", err)
				return
			}

			if err := downloadServerJar(config.Version, librariesDir); err != nil {
				log.Println("下载mc服务端失败:", err)
				return
			}

			fmt.Println("库文件下载完成")
			if err := runInstaller(installerPath); err != nil {
				log.Println("运行安装器失败:", err)
			}
		}
		if config.Loader == "forge" && config.Download == "bmclapi" {
			installerURL := fmt.Sprintf(
				"https://bmclapi2.bangbang93.com/maven/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
				config.Version, config.LoaderVersion, config.Version, config.LoaderVersion,
			)
			installerPath := filepath.Join("./.autoinst/cache", fmt.Sprintf("forge-%s-%s-installer.jar", config.Version, config.LoaderVersion))
			fmt.Println("检测到 forge 加载器，正在从 BMCLAPI 下载:", installerURL)
			if err := downloadFile(installerURL, installerPath); err != nil {
				log.Println("下载 forge 失败:", err)
				return
			}
			fmt.Println("forge 安装器下载完成:", installerPath)

			// 提取 version.json
			versionInfo, err := extractVersionJson(installerPath)
			if err != nil {
				log.Println("提取 version.json 失败:", err)
				return
			}

			librariesDir := "./libraries"
			if err := downloadLibraries(versionInfo, librariesDir); err != nil {
				log.Println("下载库文件失败:", err)
				return
			}

			if err := downloadServerJar(config.Version, librariesDir); err != nil {
				log.Println("下载mc服务端失败:", err)
				return
			}

			fmt.Println("库文件下载完成")
			if err := runInstaller(installerPath); err != nil {
				log.Println("运行安装器失败:", err)
			}
		}
	} else if os.IsNotExist(err) {
		log.Println("inst.json 文件不存在")
	} else {
		log.Println("无法访问 inst.json 文件:", err)
	}
}
