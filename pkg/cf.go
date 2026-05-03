package pkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/autoinst/AutoInstall/core"
)

type CurseForgeManifest struct {
	Minecraft struct {
		Version    string `json:"version"`
		ModLoaders []struct {
			ID      string `json:"id"`
			Primary bool   `json:"primary"`
		} `json:"modLoaders"`
	} `json:"minecraft"`
	Overrides string `json:"overrides"`
	Files     []struct {
		ProjectID int  `json:"projectID"`
		FileID    int  `json:"fileID"`
		Required  bool `json:"required"`
	} `json:"files"`
}

var cfapiKey string

type CFError struct {
	StatusCode int
	Body       string
}

func (e *CFError) Error() string {
	return fmt.Sprintf("响应异常: %d %s", e.StatusCode, e.Body)
}

func resolveCFDownloadURL(projectID, fileID int, maxRetries int) (string, error) {
	if cfapiKey == "" {
		return "", fmt.Errorf("缺少 CF_API_KEY")
	}
	maxRetries = core.NormalizeRetries(maxRetries)
	url := fmt.Sprintf("https://api.curseforge.com/v1/mods/%d/files/%d/download-url", projectID, fileID)
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("x-api-key", cfapiKey)
		req.Header.Set("User-Agent", "autoinst/1.3.0")
		resp, err := core.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			core.Logf("获取 CurseForge 直链失败 %d/%d: %v\n", i+1, maxRetries, err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = &CFError{StatusCode: resp.StatusCode, Body: string(b)}
			core.Logf("获取 CurseForge 直链失败 %d/%d: %v\n", i+1, maxRetries, lastErr)
			continue
		}
		var out struct {
			Data string `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			resp.Body.Close()
			lastErr = err
			core.Logf("解析 CurseForge 直链失败 %d/%d: %v\n", i+1, maxRetries, err)
			continue
		}
		resp.Body.Close()
		if out.Data == "" {
			lastErr = fmt.Errorf("CF API 未返回下载地址")
			core.Logf("获取 CurseForge 直链失败 %d/%d: %v\n", i+1, maxRetries, lastErr)
			continue
		}
		return out.Data, nil
	}
	return "", fmt.Errorf("多次获取 CurseForge 直链失败 (共 %d 次): %w", maxRetries, lastErr)
}

func curseForgeDownloadFilename(downloadURL string, fileID int) string {
	filename := fmt.Sprintf("%d.jar", fileID)
	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return filename
	}
	base := path.Base(parsed.EscapedPath())
	if base == "." || base == "/" || base == "" {
		return filename
	}
	decoded, err := url.PathUnescape(base)
	if err != nil || decoded == "" {
		return filename
	}
	return filepath.Base(decoded)
}

func CurseForge(file string, MaxCon int, Args string, bundleName string, MaxRetries int, baseConfig core.InstConfig) error {
	MaxRetries = core.NormalizeRetries(MaxRetries)
	mf := "manifest.json"
	if file != "" && strings.HasSuffix(strings.ToLower(file), ".json") {
		mf = file
	}
	mfPath := filepath.Join("./", mf)
	mfFile, err := os.Open(mfPath)
	if err != nil {
		return fmt.Errorf("未找到 manifest.json，停止 CurseForge 安装流程: %w", err)
	}
	defer mfFile.Close()

	var manifest CurseForgeManifest
	if err := json.NewDecoder(mfFile).Decode(&manifest); err != nil {
		return fmt.Errorf("解析 manifest.json 失败: %w", err)
	}

	overridesDir := manifest.Overrides
	if overridesDir == "" {
		overridesDir = "overrides"
	}
	if err := moveOverrides(filepath.Join("./", overridesDir)); err != nil {
		return fmt.Errorf("移动 overrides 文件失败: %w", err)
	}

	inst := modpackBaseConfig(baseConfig, MaxRetries)
	inst.Version = manifest.Minecraft.Version

	loaderID := ""
	if len(manifest.Minecraft.ModLoaders) > 0 {
		for _, ml := range manifest.Minecraft.ModLoaders {
			if ml.Primary {
				loaderID = ml.ID
				break
			}
		}
		if loaderID == "" {
			loaderID = manifest.Minecraft.ModLoaders[0].ID
		}
	}

	if strings.HasPrefix(strings.ToLower(loaderID), "neoforge-") {
		inst.Loader = "neoforge"
		inst.LoaderVersion = strings.TrimPrefix(loaderID, "neoforge-")
	} else if strings.HasPrefix(strings.ToLower(loaderID), "forge-") {
		inst.Loader = "forge"
		inst.LoaderVersion = strings.TrimPrefix(loaderID, "forge-")
	} else if strings.HasPrefix(strings.ToLower(loaderID), "fabric-") {
		inst.Loader = "fabric"
		inst.LoaderVersion = strings.TrimPrefix(loaderID, "fabric-")
	} else {
		if loaderID == "" {
			inst.Loader = "vanilla"
			inst.LoaderVersion = ""
		} else {
			inst.Loader = "fabric"
			inst.LoaderVersion = loaderID
		}
	}
	jsonData, err := json.MarshalIndent(inst, "", "  ")
	if err != nil {
		return fmt.Errorf("生成 inst.json 失败: %w", err)
	}
	if err := os.WriteFile("inst.json", jsonData, 0777); err != nil {
		return fmt.Errorf("写入 inst.json 失败: %w", err)
	}

	if len(manifest.Files) == 0 {
		runInstalledModFilter(bundleName)
		return nil
	}

	var wg sync.WaitGroup
	maxConcurrency := 24
	if MaxCon > 0 {
		maxConcurrency = MaxCon
	}
	semaphore := make(chan struct{}, maxConcurrency)
	errChan := make(chan error, len(manifest.Files))

	modsDir := filepath.Join(".", "mods")
	if err := os.MkdirAll(modsDir, os.ModePerm); err != nil {
		return fmt.Errorf("创建 mods 目录失败: %w", err)
	}

	for _, mf := range manifest.Files {
		if !mf.Required {
			continue
		}
		wg.Add(1)
		semaphore <- struct{}{}

		go func(entry struct {
			ProjectID int  `json:"projectID"`
			FileID    int  `json:"fileID"`
			Required  bool `json:"required"`
		}) {
			defer func() { <-semaphore; wg.Done() }()

			url, err := resolveCFDownloadURL(entry.ProjectID, entry.FileID, MaxRetries)
			if err != nil {
				apiUrl := fmt.Sprintf("https://api.curseforge.com/v1/mods/%d/files/%d/download-url", entry.ProjectID, entry.FileID)
				respBody := err.Error()
				if cfErr, ok := err.(*CFError); ok {
					respBody = cfErr.Body
				}
				core.RecordError(apiUrl, err, respBody)
				errChan <- fmt.Errorf("获取直链失败(Project %d, File %d): %v", entry.ProjectID, entry.FileID, err)
				return
			}

			filename := curseForgeDownloadFilename(url, entry.FileID)
			dst := filepath.Join(modsDir, filename)
			if err := os.MkdirAll(filepath.Dir(dst), os.ModePerm); err != nil {
				errChan <- err
				return
			}
			core.Log("尝试下载:", url)
			if err := core.DownloadFileRetry(url, dst, MaxRetries); err != nil {
				core.RecordError(url, err, "Download failed")
				errChan <- fmt.Errorf("下载失败(Project %d, File %d): %v", entry.ProjectID, entry.FileID, err)
				return
			}
		}(mf)
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
	if err := errors.Join(errs...); err != nil {
		return err
	}

	runInstalledModFilter(bundleName)
	return nil
}
