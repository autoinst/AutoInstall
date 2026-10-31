package pkg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/autoinst/AutoInstall/core"
	"github.com/autoinst/AutoInstall/internal/workspace"
)

func DownloadLibrariesAndServerJar(versionInfo core.VersionInfo, version, loader, librariesDir string, maxConnections int, downloadapi string, maxRetries int) error {
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := DownloadLibraries(versionInfo, librariesDir, maxConnections, downloadapi, maxRetries); err != nil {
			errChan <- fmt.Errorf("下载库文件失败: %w", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := DownloadServerJar(version, loader, librariesDir, downloadapi, maxRetries, versionInfo.ServerJarPath); err != nil {
			errChan <- fmt.Errorf("下载 mc 服务端失败: %w", err)
		}
	}()

	wg.Wait()
	close(errChan)

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func DownloadLibraries(versionInfo core.VersionInfo, librariesDir string, maxConnections int, downloadapi string, maxRetries int) error {
	// Classification is local to this prefetch pass; do not mutate the caller's slice.
	versionInfo.Libraries = append([]core.Library(nil), versionInfo.Libraries...)
	if err := core.AnalyzeServerMetadata(&versionInfo); err != nil {
		return fmt.Errorf("分析服务端库元数据失败: %w", err)
	}
	for _, warning := range versionInfo.MetadataWarnings {
		core.Logf("警告: %s\n", warning)
	}
	if err := os.MkdirAll(librariesDir, os.ModePerm); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	if maxConnections <= 0 {
		maxConnections = 4
	}

	sem := make(chan struct{}, maxConnections)
	errChan := make(chan error, len(versionInfo.Libraries))
	var wg sync.WaitGroup

	type libraryTask struct {
		lib             core.Library
		url, path, sha1 string
	}
	tasks := make([]libraryTask, 0, len(versionInfo.Libraries))
	seen := make(map[string]int)
	for _, lib := range versionInfo.Libraries {
		if lib.Kind != core.LibraryRemote {
			core.Logf("跳过库预下载 %s (%s): %s\n", lib.Name, lib.Kind, lib.Reason)
			continue
		}
		officialURL, artifactPath, sha1 := resolveLibraryArtifact(lib)
		if officialURL == "" || artifactPath == "" {
			return fmt.Errorf("必需远程库 %s 缺少有效 URL 或路径", lib.Name)
		}
		if index, exists := seen[artifactPath]; exists {
			previous := &tasks[index]
			if previous.sha1 != "" && sha1 != "" && !strings.EqualFold(previous.sha1, sha1) {
				return fmt.Errorf("库路径 %s 的 SHA1 冲突", artifactPath)
			}
			if previous.sha1 == "" {
				previous.sha1 = sha1
			}
			continue
		}
		seen[artifactPath] = len(tasks)
		tasks = append(tasks, libraryTask{lib, officialURL, artifactPath, sha1})
	}
	for _, task := range tasks {
		lib, officialURL, artifactPath, sha1 := task.lib, task.url, task.path, task.sha1
		currentURL, fallbackURL := downloadURLPair(officialURL, downloadapi)
		filePath := filepath.Join(librariesDir, artifactPath)

		wg.Add(1)
		go func(lib core.Library, currentURL, fallbackURL, filePath, sha1 string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if _, err := workspace.SafeJoin(librariesDir, artifactPath); err != nil {
				errChan <- err
				return
			}
			if info, err := os.Stat(filePath); err == nil {
				if !info.Mode().IsRegular() {
					errChan <- fmt.Errorf("库目标不是普通文件: %s", filePath)
					return
				}
				if info.Size() > 0 && core.VerifyFileHash(filePath, "sha1", sha1) == nil {
					core.Logf("已存在且校验通过: %s\n", filePath)
					return
				}
			} else if !os.IsNotExist(err) {
				errChan <- err
				return
			}
			dir, err := os.MkdirTemp(librariesDir, ".download-")
			if err != nil {
				errChan <- err
				return
			}
			defer os.RemoveAll(dir)
			temp := filepath.Join(dir, "payload")
			if _, err = core.DownloadCandidates(context.Background(), core.HTTPClient, []string{currentURL, fallbackURL}, temp, maxRetries, map[string]string{"sha1": sha1}); err != nil {
				errChan <- fmt.Errorf("下载库 %s 失败: %w", lib.Name, err)
				return
			}
			if err = os.MkdirAll(filepath.Dir(filePath), 0755); err == nil {
				_, err = workspace.SafeJoin(librariesDir, artifactPath)
			}
			if err != nil {
				errChan <- err
				return
			}
			// Replace a corrupt cache entry only after a verified replacement exists.
			if err = os.Rename(temp, filePath); err != nil {
				errChan <- err
				return
			}
			core.Log("下载完成:", filePath)
		}(lib, currentURL, fallbackURL, filePath, sha1)
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

func mavenRelPathFromNameWithFile(name, fileName string) string {
	artifact, _ := core.LibraryArtifactPath(core.Library{Name: name, FilePath: fileName})
	return artifact
}

func resolveLibraryArtifact(lib core.Library) (url, path, sha1 string) {
	url = strings.TrimSpace(lib.Downloads.Artifact.URL)
	path, _ = core.LibraryArtifactPath(lib)
	sha1 = strings.TrimSpace(lib.Downloads.Artifact.SHA1)
	if sha1 == "" && len(lib.Checksums) > 0 {
		sha1 = strings.TrimSpace(lib.Checksums[0])
	}

	if lib.Kind != "" && lib.Kind != core.LibraryRemote {
		return "", path, sha1
	}
	// Only legacy Maven entries may synthesize a URL. A modern empty or absent
	// artifact URL is an installer responsibility, not a libraries.minecraft.net URL.
	if url == "" && (lib.DownloadsPresent || lib.ArtifactPresent || lib.ArtifactURLPresent || lib.Downloads.Artifact.Path != "" || lib.Downloads.Artifact.SHA1 != "") {
		return "", path, sha1
	}
	if url == "" && path != "" {
		base := strings.TrimSpace(lib.URL)
		if base == "" {
			base = "https://libraries.minecraft.net/"
		}
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		url = base + path
	}
	return url, path, sha1
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
