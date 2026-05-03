package pkg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/autoinst/AutoInstall/core"
)

func DownloadLibraries(versionInfo core.VersionInfo, librariesDir string, maxConnections int, downloadapi string, maxRetries int) error {
	if err := os.MkdirAll(librariesDir, os.ModePerm); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	if maxConnections <= 0 {
		maxConnections = 4
	}

	sem := make(chan struct{}, maxConnections)
	errChan := make(chan error, len(versionInfo.Libraries))
	var wg sync.WaitGroup

	for _, lib := range versionInfo.Libraries {
		if lib.Downloads.Artifact.URL == "" {
			core.Logf("跳过库文件 %s: 未提供下载 URL\n", lib.Name)
			continue
		}

		officialURL := lib.Downloads.Artifact.URL
		currentURL, fallbackURL := downloadURLPair(officialURL, downloadapi)
		if currentURL == "" {
			core.Logf("警告: 处理后 URL 仍为空，跳过库 %s\n", lib.Name)
			continue
		}
		filePath := filepath.Join(librariesDir, lib.Downloads.Artifact.Path)

		wg.Add(1)
		go func(lib core.Library, currentURL, fallbackURL, filePath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if _, err := os.Stat(filePath); err == nil {
				if err := core.VerifyFileHash(filePath, "sha1", lib.Downloads.Artifact.SHA1); err == nil {
					core.Logf("已存在且校验通过: %s\n", filePath)
					return
				}
				core.Logf("文件 %s 校验失败 (或无法校验)，重新下载...\n", filePath)
				if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
					errChan <- fmt.Errorf("删除校验失败文件 %s 失败: %w", filePath, err)
					return
				}
			}

			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				errChan <- fmt.Errorf("无法创建目录: %w", err)
				return
			}
			if err := core.DownloadFileWithFallback(currentURL, fallbackURL, filePath, maxRetries); err != nil {
				errChan <- fmt.Errorf("下载库 %s 失败: %w", lib.Name, err)
				return
			}
			if err := core.VerifyFileHash(filePath, "sha1", lib.Downloads.Artifact.SHA1); err != nil {
				_ = os.Remove(filePath)
				errChan <- fmt.Errorf("下载库 %s 后校验失败: %w", lib.Name, err)
				return
			}
			core.Log("下载完成:", filePath)
		}(lib, currentURL, fallbackURL, filePath)
	}
	wg.Wait()
	close(errChan)

	var errs []error
	for err := range errChan {
		if err != nil {
			core.Log("下载出错:", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func downloadURLPair(officialURL string, downloadapi string) (string, string) {
	mirrorURL := mirrorDownloadURL(officialURL)
	if downloadapi == "bmclapi" {
		return mirrorURL, officialURL
	}
	return officialURL, mirrorURL
}

func mirrorDownloadURL(officialURL string) string {
	url := officialURL
	replacements := []struct {
		old string
		new string
	}{
		{"https://maven.minecraftforge.net/", "https://bmclapi2.bangbang93.com/maven/"},
		{"https://maven.fabricmc.net/", "https://bmclapi2.bangbang93.com/maven/"},
		{"https://maven.neoforged.net/releases/", "https://bmclapi2.bangbang93.com/maven/"},
		{"https://libraries.minecraft.net/", "https://bmclapi2.bangbang93.com/maven/"},
		{"https://repo1.maven.org/maven2/", "https://bmclapi2.bangbang93.com/maven/"},
	}
	for _, replacement := range replacements {
		if strings.HasPrefix(url, replacement.old) {
			return strings.Replace(url, replacement.old, replacement.new, 1)
		}
	}
	return officialURL
}
