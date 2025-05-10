package pkg

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/autoinst/AutoInstall/core" // 确保导入 core 包，如果 VersionInfo 在 core 包中定义
)

func DownloadLibraries(versionInfo core.VersionInfo, librariesDir string, maxConnections int) error {
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
		go func(lib core.Library, url, filePath string) {
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
			if err := core.DownloadFile(url, filePath); err != nil {
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
