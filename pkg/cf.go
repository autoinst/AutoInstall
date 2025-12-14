package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

// resolveCFDownloadURL 使用 CurseForge API 获取可用直链
// 需要环境变量 CF_API_KEY，可在 https://console.curseforge.com/ 申请
func resolveCFDownloadURL(projectID, fileID int) (string, error) {
	if cfapiKey == "" {
		return "", fmt.Errorf("缺少 CF_API_KEY")
		os.Exit(128)
	}
	// 直接获取下载直链
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.curseforge.com/v1/mods/%d/files/%d/download-url", projectID, fileID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-api-key", cfapiKey)
	req.Header.Set("User-Agent", "autoinst/1.3.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", &CFError{StatusCode: resp.StatusCode, Body: string(b)}
	}
	var out struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Data == "" {
		return "", fmt.Errorf("CF API 未返回下载地址")
	}
	return out.Data, nil
}

func CurseForge(file string, MaxCon int, Args string) {
	mf := "manifest.json"
	if file != "" && strings.HasSuffix(strings.ToLower(file), ".json") {
		mf = file
	}
	mfPath := filepath.Join("./", mf)
	mfFile, err := os.Open(mfPath)
	if err != nil {
		core.Log("未找到 manifest.json，停止 CurseForge 安装流程")
		return
	}
	defer mfFile.Close()

	var manifest CurseForgeManifest
	if err := json.NewDecoder(mfFile).Decode(&manifest); err != nil {
		core.Log("解析 manifest.json 失败:", err)
		return
	}

	overridesDir := manifest.Overrides
	if overridesDir == "" {
		overridesDir = "overrides"
	}
	if err := moveOverrides(filepath.Join("./", overridesDir)); err != nil {
		core.Log("移动 overrides 文件失败:", err)
		return
	}

	inst := core.InstConfig{
		Version:        manifest.Minecraft.Version,
		Download:       "bmclapi",
		MaxConnections: 32,
		Argsment:       "-Xmx{maxmen}M -Xms{maxmen}M -XX:+AlwaysPreTouch -XX:+DisableExplicitGC -XX:+ParallelRefProcEnabled -XX:+PerfDisableSharedMem -XX:+UnlockExperimentalVMOptions -XX:+UseG1GC -XX:G1HeapRegionSize=8M -XX:G1HeapWastePercent=5 -XX:G1MaxNewSizePercent=40 -XX:G1MixedGCCountTarget=4 -XX:G1MixedGCLiveThresholdPercent=90 -XX:G1NewSizePercent=30 -XX:G1RSetUpdatingPauseTimePercent=5 -XX:G1ReservePercent=20 -XX:InitiatingHeapOccupancyPercent=15 -XX:MaxGCPauseMillis=200 -XX:MaxTenuringThreshold=1 -XX:SurvivorRatio=32 -Dusing.aikars.flags=https://mcflags.emc.gs -Daikars.new.flags=true",
	}

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
		core.Log("生成 inst.json 失败:", err)
		return
	}
	if err := os.WriteFile("inst.json", jsonData, 0777); err != nil {
		core.Log("写入 inst.json 失败:", err)
		return
	}

	if len(manifest.Files) == 0 {
		return
	}

	DownloadWg.Add(1)
	go func() {
		defer DownloadWg.Done()
		var wg sync.WaitGroup
		maxConcurrency := 24
		if MaxCon > 0 {
			maxConcurrency = MaxCon
		}
		semaphore := make(chan struct{}, maxConcurrency)
		errChan := make(chan error, len(manifest.Files))

		modsDir := filepath.Join(".", "mods")
		_ = os.MkdirAll(modsDir, os.ModePerm)

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

				url, err := resolveCFDownloadURL(entry.ProjectID, entry.FileID)
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

				segs := strings.Split(url, "/")
				filename := fmt.Sprintf("%d.jar", entry.FileID)
				if len(segs) > 0 && segs[len(segs)-1] != "" {
					filename = segs[len(segs)-1]
				}
				dst := filepath.Join(modsDir, filename)
				if err := os.MkdirAll(filepath.Dir(dst), os.ModePerm); err != nil {
					errChan <- err
					return
				}
				core.Log("尝试下载:", url)
				if err := core.DownloadFile(url, dst); err != nil {
					core.RecordError(url, err, "Download failed")
					errChan <- fmt.Errorf("下载失败(Project %d, File %d): %v", entry.ProjectID, entry.FileID, err)
					return
				}
			}(mf)
		}

		wg.Wait()
		close(errChan)
		for err := range errChan {
			if err != nil {
				core.Log("下载出错:", err)
			}
		}
	}()
}
