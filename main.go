package main

import (
	"archive/zip"
	"crypto/sha1"
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
	"sync"
	"time"
)

// InstConfig 结构体表示 inst.json 的内容
type InstConfig struct {
	Version        string `json:"version"`
	Loader         string `json:"loader"`
	LoaderVersion  string `json:"loaderVersion"`
	Download       string `json:"download"`
	MaxConnections int    `json:"maxconnections"`
}

// Config 定义配置文件的结构
type Config struct {
	MaxConnections int `json:"maxconnections"`
}

// Library 定义库的结构
type Library struct {
	Name      string `json:"name"`
	Downloads struct {
		Artifact struct {
			URL  string `json:"url"`
			Path string `json:"path"`
			SHA1 string `json:"sha1"`
		} `json:"artifact"`
	} `json:"downloads"`
}

// VersionInfo 定义 version.json 文件的结构
type VersionInfo struct {
	Libraries []Library `json:"libraries"`
}

var gitversion string

func downloadServerJar(version, loader, librariesDir string) error {
	downloadURL := fmt.Sprintf("https://bmclapi2.bangbang93.com/version/%s/server", version)
	var serverFileName string

	if loader == "forge" {
		serverFileName = fmt.Sprintf("server-%s-bundled.jar", version)
	} else if loader == "fabric" {
		serverFileName = "server.jar"
	} else {
		serverFileName = fmt.Sprintf("server-%s.jar", version)
	}

	var serverPath string
	if loader == "fabric" {
		serverPath = filepath.Join(".", serverFileName)
	} else {
		serverPath = filepath.Join(librariesDir, "net", "minecraft", "server", version, serverFileName)
	}

	if err := os.MkdirAll(filepath.Dir(serverPath), os.ModePerm); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	if err := downloadFile(downloadURL, serverPath); err != nil {
		return fmt.Errorf("无法下载服务端文件 %s: %v", serverPath, err)
	}
	fmt.Println("下载完成 Minecraft 服务端:", serverPath)

	// 感谢SBforge改得到处都是
	if loader == "forge" {
		var symlinkPath string
		if version < "1.16.5" {
			symlinkPath = fmt.Sprintf("./minecraft_server.%s.jar", version)
		} else {
			symlinkPath = filepath.Join(librariesDir, "net", "minecraft", "server", version, fmt.Sprintf("server-%s.jar", version))
		}

		if err := os.Symlink(serverPath, symlinkPath); err != nil {
			return fmt.Errorf("无法创建符号链接 %s -> %s: %v", symlinkPath, serverPath, err)
		}
		fmt.Println("符号链接已创建:", symlinkPath, "->", serverPath)
	}
	return nil
}

func runInstaller(installerPath string, loader string, version string, loaderVersion string, Download string) error {
	var cmd *exec.Cmd
	if Download == "bmclapi" {
		if loader == "forge" {
			cmd = exec.Command("java", "-jar", installerPath, "--installServer", "--mirror", "https://bmclapi2.bangbang93.com/maven/")
		} else if loader == "fabric" {
			cmd = exec.Command(
				"java", "-jar", installerPath, "server",
				"-mavenurl", "https://bmclapi2.bangbang93.com/maven/",
				"-metaurl", "https://bmclapi2.bangbang93.com/fabric-meta/",
				"-mcversion", version,
				"-loader", loaderVersion,
			)
		} else {
			cmd = exec.Command("java", "-jar", installerPath)
		}
	} else {
		if loader == "forge" {
			cmd = exec.Command("java", "-jar", installerPath, "--installServer")
		} else if loader == "fabric" {
			cmd = exec.Command(
				"java", "-jar", installerPath, "server",
				"-mcversion", version,
				"-loader", loaderVersion,
			)
		} else {
			cmd = exec.Command("java", "-jar", installerPath)
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

// 下载文件的函数
func downloadFile(url, filePath string) error {
	const maxRetries = 3
	var err error
	for i := 0; i < maxRetries; i++ {
		resp, err := http.Get(url)
		if err != nil {
			fmt.Printf("下载文件尝试 %d/%d 失败: %v\n", i+1, maxRetries, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			fmt.Printf("文件未找到，跳过下载: %s\n", url)
			return err
		}

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("下载文件失败，状态码: %d\n", resp.StatusCode)
			continue
		}

		file, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("无法创建文件: %v", err)
		}
		defer file.Close()

		// 使用 io.TeeReader 报告下载进度
		body := resp.Body
		totalBytes, _ := io.Copy(io.Discard, body) // 获取文件大小
		resp.Body.Close()                          // 关闭先前的 body
		resp, err = http.Get(url)                  // 重新发起请求
		if err != nil {
			return fmt.Errorf("重新获取文件失败: %v", err)
		}
		defer resp.Body.Close()
		body = resp.Body

		reader := &ProgressReader{
			Reader:          body,
			Total:           totalBytes,
			FilePath:        filePath,
			UpdateInterval:  3, // 每 3 秒更新一次
			lastUpdatedTime: 0,
		}

		_, err = io.Copy(file, reader)
		if err != nil {
			fmt.Printf("写入文件尝试 %d/%d 失败: %v\n", i+1, maxRetries, err)
			continue
		}

		return nil
	}
	return fmt.Errorf("下载文件失败，经过 %d 次尝试: %v", maxRetries, err)
}

// ProgressReader 用于跟踪 io.Reader 的进度
type ProgressReader struct {
	Reader          io.ReadCloser
	Total           int64
	Current         int64
	FilePath        string
	UpdateInterval  int64 // 更新间隔，单位秒
	lastUpdatedTime int64
}

// Read 实现了 io.Reader 接口
func (pr *ProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.Reader.Read(p)
	pr.Current += int64(n)

	currentTime := time.Now().Unix()
	if currentTime-pr.lastUpdatedTime >= pr.UpdateInterval || err == io.EOF {
		pr.lastUpdatedTime = currentTime
		progress := float64(pr.Current) / float64(pr.Total) * 100
		fmt.Printf("下载进度: %.2f%% (%s)\n", progress, pr.FilePath)
	}

	return
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
				log.Printf("警告: 无法打开 version.json 文件: %v", err)
				continue
			}
			defer rc.Close()
			if err := json.NewDecoder(rc).Decode(&versionInfo); err != nil {
				log.Printf("警告: 无法解析 version.json: %v", err)
				continue
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

// 计算文件的 SHA1 哈希值
func computeSHA1(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha1.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// 下载库文件（支持 SHA1 校验）
func downloadLibraries(versionInfo VersionInfo, librariesDir string, maxConnections int) error {
	if err := os.MkdirAll(librariesDir, os.ModePerm); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	sem := make(chan struct{}, maxConnections) // 控制并发数
	var wg sync.WaitGroup

	for _, lib := range versionInfo.Libraries {
		if lib.Downloads.Artifact.URL == "" {
			fmt.Printf("跳过库文件 %s: 未提供下载 URL\n", lib.Name)
			continue
		}

		url := lib.Downloads.Artifact.URL
		url = strings.Replace(url, "https://maven.minecraftforge.net/", "https://bmclapi2.bangbang93.com/maven/", 1)
		url = strings.Replace(url, "https://maven.fabricmc.net/", "https://bmclapi2.bangbang93.com/maven/", 1)
		url = strings.Replace(url, "https://maven.neoforged.net/releases/", "https://bmclapi2.bangbang93.com/maven/", 1)
		url = strings.Replace(url, "https://libraries.minecraft.net/", "https://bmclapi2.bangbang93.com/maven/", 1)

		if url == "" {
			fmt.Printf("警告: 处理后 URL 仍为空，跳过库 %s\n", lib.Name)
			continue
		}
		filePath := filepath.Join(librariesDir, lib.Downloads.Artifact.Path)

		wg.Add(1)
		go func(lib Library, url, filePath string) {
			defer wg.Done()
			sem <- struct{}{} // 获取令牌
			// 校验 SHA1
			if _, err := os.Stat(filePath); err == nil {
				fileSHA1, err := computeSHA1(filePath)
				if err == nil && fileSHA1 == lib.Downloads.Artifact.SHA1 {
					fmt.Printf("已存在且校验通过: %s\n", filePath)
					<-sem // 释放令牌
					return
				} else {
					fmt.Printf("文件 %s 校验失败 (或无法校验)，重新下载...\n", filePath)
					os.Remove(filePath)
				}
			}
			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				fmt.Printf("无法创建目录: %v\n", err)
			}
			fmt.Println("正在下载:", url)
			if err := downloadFile(url, filePath); err != nil {
				fmt.Printf("下载失败 %s (%s): %v\n", lib.Name, url, err)
			} else {
				fmt.Println("下载完成:", filePath)
			}
			<-sem // 释放令牌
		}(lib, url, filePath)
	}
	wg.Wait()
	return nil
}

func findJava() (string, bool) {
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

func main() {
	if gitversion == "" {
		gitversion = "NaN"
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("AutoInstall-" + gitversion)
	}
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
		fmt.Println("AutoInstall-" + gitversion + " https://github.com/autoinst/AutoInstall")
		fmt.Println("准备安装:")
		fmt.Printf("Minecraft版本: %s\n", config.Version)
		if config.Loader != "vanilla" {
			fmt.Printf("加载器: %s\n", config.Loader)
			fmt.Printf("加载器版本: %s\n", config.LoaderVersion)
			fmt.Println("\033[31m[警告] 加载器版本过新可能会无法正常下载\033[0m")
		}
		fmt.Printf("下载源: %s\n", config.Download)

		javaPath, simpfun := findJava()
		if javaPath == "" {
			log.Println("未找到 Java，请确保已安装 Java 并设置 PATH。")
			return
		}
		fmt.Println("找到 Java 运行环境:", javaPath)
		if simpfun {
			fmt.Println("已启用 simpfun 环境")
		}
		os.MkdirAll(".autoinst/cache", os.ModePerm)
		if config.Loader == "neoforge" {
			var installerURL string
			if config.Download == "bmclapi" {
				installerURL = fmt.Sprintf(
					"https://bmclapi2.bangbang93.com/maven/net/neoforged/neoforge/%s/neoforge-%s-installer.jar",
					config.LoaderVersion, config.LoaderVersion,
				)
			} else {
				// 这里替换为官方源的URL，如果存在
				installerURL = fmt.Sprintf(
					"https://maven.neoforged.net/releases/net/neoforged/neoforge/%s/neoforge-%s-installer.jar",
					config.LoaderVersion, config.LoaderVersion,
				)
			}

			installerPath := filepath.Join("./.autoinst/cache", fmt.Sprintf("neoforge-%s-installer.jar", config.LoaderVersion))
			fmt.Println("当前为 neoforge 加载器，正在下载:", installerURL)
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
			if err := downloadLibraries(versionInfo, librariesDir, config.MaxConnections); err != nil {
				log.Println("下载库文件失败:", err)
				return
			}

			if err := downloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
				log.Println("下载mc服务端失败:", err)
				return
			}

			fmt.Println("库文件下载完成")
			if err := runInstaller(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download); err != nil {
				log.Println("运行安装器失败:", err)
			}
		}
		if config.Loader == "forge" {
			var installerURL string
			if config.Download == "bmclapi" {
				installerURL = fmt.Sprintf(
					"https://bmclapi2.bangbang93.com/maven/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
					config.Version, config.LoaderVersion, config.Version, config.LoaderVersion,
				)
			} else {
				// 这里替换为官方源的URL，如果存在
				installerURL = fmt.Sprintf(
					"https://maven.minecraftforge.net/maven/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
					config.Version, config.LoaderVersion, config.Version, config.LoaderVersion,
				)
			}
			installerPath := filepath.Join("./.autoinst/cache", fmt.Sprintf("forge-%s-%s-installer.jar", config.Version, config.LoaderVersion))
			fmt.Println("当前为 forge 加载器，正在下载:", installerURL)
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
			if err := downloadLibraries(versionInfo, librariesDir, config.MaxConnections); err != nil {
				log.Println("下载库文件失败:", err)
				return
			}

			if err := downloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
				log.Println("下载mc服务端失败:", err)
				return
			}

			fmt.Println("库文件下载完成")
			if err := runInstaller(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download); err != nil {
				log.Println("运行安装器失败:", err)
			} else {
				// 检测是否存在 forge-版本-加载器版本-universal.jar
				universalJar := fmt.Sprintf("forge-%s-%s-universal.jar", config.Version, config.LoaderVersion)
				if _, err := os.Stat(universalJar); err == nil {
					// 创建 run.sh 文件
					runScriptPath := "run.sh"
					var runCommand string
					if simpfun {
						runCommand = fmt.Sprintf("/usr/bin/jdk/jdk1.8.0_361/bin/java -jar %s", universalJar)
					} else {
						runCommand = fmt.Sprintf("java -jar %s", universalJar)
					}
					// 写入 run.sh 文件
					if err := os.WriteFile(runScriptPath, []byte(runCommand), 0755); err != nil {
						log.Printf("无法创建 run.sh 文件: %v", err)
					} else {
						fmt.Println("已创建运行脚本:", runScriptPath)
					}
				} else {
					fmt.Printf("未找到文件 %s，跳过创建运行脚本。\n", universalJar)
				}
			}
		}
		if config.Loader == "fabric" {
			var installerURL string
			installerURL = "https://maven.fabricmc.net/net/fabricmc/fabric-installer/1.0.1/fabric-installer-1.0.1.jar"
			installerPath := filepath.Join("./.autoinst/cache", "fabric-installer-1.0.1.jar")
			fmt.Println("当前为 fabric 加载器，正在下载:", installerURL)
			if err := downloadFile(installerURL, installerPath); err != nil {
				log.Println("下载 fabric 失败:", err)
				return
			}
			fmt.Println("fabric 安装器下载完成:", installerPath)
			librariesDir := "./libraries"
			if err := downloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
				log.Println("下载mc服务端失败:", err)
				return
			}
			fmt.Println("服务端下载完成")
			if err := runInstaller(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download); err != nil {
				log.Println("运行安装器失败:", err)
				return
			}
			// 创建 run.sh 文件
			runScriptPath := "run.sh"
			var javaPath string
			if simpfun {
				// 根据版本号选择 Java 路径
				if config.Version < "1.17" {
					javaPath = "/usr/bin/jdk/jdk1.8.0_361/bin/java"
				} else if config.Version >= "1.17" && config.Version <= "1.20.3" {
					javaPath = "/usr/bin/jdk/jdk-17.0.6/bin/java"
				} else {
					javaPath = "/usr/bin/jdk/jdk-21.0.2/bin/java"
				}
			} else {
				javaPath = "java"
			}
			// 拼接运行命令
			runCommand := fmt.Sprintf("%s -jar fabric-server-launch.jar", javaPath)
			// 写入 run.sh 文件
			if err := os.WriteFile(runScriptPath, []byte(runCommand), 0777); err != nil {
				log.Printf("无法创建 run.sh 文件: %v", err)
			} else {
				fmt.Println("已创建运行脚本:", runScriptPath)
			}
		}
		if config.Loader == "vanilla" {
			librariesDir := "./libraries"
			if err := downloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
				log.Println("下载mc服务端失败:", err)
				return
			}
			fmt.Println("服务端下载完成")
			// 创建 run.sh 文件
			runScriptPath := "run.sh"
			var javaPath string
			if simpfun {
				// 根据版本号选择 Java 路径
				if config.Version < "1.17" {
					javaPath = "/usr/bin/jdk/jdk1.8.0_361/bin/java"
				} else if config.Version >= "1.17" && config.Version <= "1.20.3" {
					javaPath = "/usr/bin/jdk/jdk-17.0.6/bin/java"
				} else {
					javaPath = "/usr/bin/jdk/jdk-21.0.2/bin/java"
				}
			} else {
				javaPath = "java"
			}
			// 拼接运行命令
			runCommand := fmt.Sprintf("%s -jar server.jar", javaPath)
			// 写入 run.sh 文件
			if err := os.WriteFile(runScriptPath, []byte(runCommand), 0777); err != nil {
				log.Printf("无法创建 run.sh 文件: %v", err)
			} else {
				fmt.Println("已创建运行脚本:", runScriptPath)
			}
		}
	} else if os.IsNotExist(err) {
		log.Println("inst.json 文件不存在")
	} else {
		log.Println("无法访问 inst.json 文件:", err)
	}
}
