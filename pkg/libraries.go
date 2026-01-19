package pkg

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/autoinst/AutoInstall/core"
)

func DownloadLibraries(versionInfo core.VersionInfo, librariesDir string, maxConnections int, downloadapi string) error {
	if err := os.MkdirAll(librariesDir, os.ModePerm); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	sem := make(chan struct{}, maxConnections)
	var wg sync.WaitGroup

	for _, lib := range versionInfo.Libraries {
		if lib.Downloads.Artifact.URL == "" {
			core.Logf("跳过库文件 %s: 未提供下载 URL\n", lib.Name)
			continue
		}

		originalURL := lib.Downloads.Artifact.URL
		url := originalURL
		if downloadapi == "bmclapi" {
			url = strings.Replace(url, "https://maven.minecraftforge.net/", "https://bmclapi2.bangbang93.com/maven/", 1)
			url = strings.Replace(url, "https://maven.fabricmc.net/", "https://bmclapi2.bangbang93.com/maven/", 1)
			url = strings.Replace(url, "https://maven.neoforged.net/releases/", "https://bmclapi2.bangbang93.com/maven/", 1)
			url = strings.Replace(url, "https://libraries.minecraft.net/", "https://bmclapi2.bangbang93.com/maven/", 1)
		}

		if url == "" {
			core.Logf("警告: 处理后 URL 仍为空，跳过库 %s\n", lib.Name)
			continue
		}
		filePath := filepath.Join(librariesDir, lib.Downloads.Artifact.Path)

		wg.Add(1)
		go func(lib core.Library, url, originalURL, filePath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// 校验 SHA1
			if _, err := os.Stat(filePath); err == nil {
				fileSHA1, err := computeSHA1(filePath)
				if err == nil && fileSHA1 == lib.Downloads.Artifact.SHA1 {
					core.Logf("已存在且校验通过: %s\n", filePath)
					return
				} else {
					core.Logf("文件 %s 校验失败 (或无法校验)，重新下载...\n", filePath)
					os.Remove(filePath)
				}
			}

			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				core.Logf("无法创建目录: %v\n", err)
				return
			}
			err := core.DownloadFile(url, filePath)
			if err != nil {
				core.Logf("下载失败 %s: %v\n", lib.Name, err)
				core.Log("尝试使用原始链接下载:", originalURL)
				if err := core.DownloadFile(originalURL, filePath); err != nil {
					core.Logf("原始链接下载也失败 %s: %v\n", lib.Name, err)
				} else {
					core.Log("使用原始链接下载完成:", filePath)
				}
			} else {
				core.Log("下载完成:", filePath)
			}
		}(lib, url, originalURL, filePath)
	}
	wg.Wait()
	return nil
}

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
