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

// resolveCFDownloadURL 使用 CurseForge API 获取可用直链
// 需要环境变量 CF_API_KEY，可在 https://console.curseforge.com/ 申请
func resolveCFDownloadURL(projectID, fileID int) (string, error) {
	apiKey := os.Getenv("CF_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("缺少 CF_API_KEY")
	}
	// 直接获取下载直链
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.curseforge.com/v1/mods/%d/files/%d/download-url", projectID, fileID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-api-key", apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("CF API 响应异常: %d %s", resp.StatusCode, string(b))
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
	// 假定压缩包已在 modpack.go 中解压
	// 1) 读取 manifest.json
	mf := "manifest.json"
	if file != "" && strings.HasSuffix(strings.ToLower(file), ".json") {
		mf = file
	}
	mfPath := filepath.Join("./", mf)
	mfFile, err := os.Open(mfPath)
	if err != nil {
		fmt.Println("未找到 manifest.json，停止 CurseForge 安装流程")
		os.Exit(0)
	}
	defer mfFile.Close()

	var manifest CurseForgeManifest
	if err := json.NewDecoder(mfFile).Decode(&manifest); err != nil {
		panic(fmt.Errorf("解析 manifest.json 失败: %w", err))
	}

	// 2) 迁移 overrides 内容到根目录
	overridesPath := filepath.Join("./", manifest.Overrides)
	if manifest.Overrides == "" {
		overridesPath = filepath.Join("./", "overrides")
	}
	if stat, statErr := os.Stat(overridesPath); statErr == nil && stat.IsDir() {
		err = filepath.Walk(overridesPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !info.IsDir() {
				relPath, err := filepath.Rel(overridesPath, path)
				if err != nil {
					return err
				}
				destPath := filepath.Join("./", relPath)
				if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
					return err
				}
				if err := os.Rename(path, destPath); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			panic(fmt.Sprintf("移动 overrides 文件失败: %v", err))
		}
		_ = os.RemoveAll(overridesPath)
	}

	// 3) 生成 inst.json（Minecraft 版本 + 加载器信息）
	inst := core.InstConfig{
		Version:        manifest.Minecraft.Version,
		Download:       "bmclapi",
		MaxConnections: 32,
		Argsment:       "-Xmx{maxmen}M -Xms{maxmen}M -XX:+AlwaysPreTouch -XX:+DisableExplicitGC -XX:+ParallelRefProcEnabled -XX:+PerfDisableSharedMem -XX:+UnlockExperimentalVMOptions -XX:+UseG1GC -XX:G1HeapRegionSize=8M -XX:G1HeapWastePercent=5 -XX:G1MaxNewSizePercent=40 -XX:G1MixedGCCountTarget=4 -XX:G1MixedGCLiveThresholdPercent=90 -XX:G1NewSizePercent=30 -XX:G1RSetUpdatingPauseTimePercent=5 -XX:G1ReservePercent=20 -XX:InitiatingHeapOccupancyPercent=15 -XX:MaxGCPauseMillis=200 -XX:MaxTenuringThreshold=1 -XX:SurvivorRatio=32 -Dusing.aikars.flags=https://mcflags.emc.gs -Daikars.new.flags=true",
	}
	// 从 modLoaders 选择 primary 或第一个
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
	// 常见格式: forge-<ver> / fabric-<loader>
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
		// 默认回退 fabric（部分清单可能只有 minecraft 原版）
		if loaderID == "" {
			inst.Loader = "vanilla"
			inst.LoaderVersion = ""
		} else {
			// 未识别时尽量按原样放入 fabric 字段，避免阻断
			inst.Loader = "fabric"
			inst.LoaderVersion = loaderID
		}
	}
	jsonData, err := json.MarshalIndent(inst, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("inst.json", jsonData, 0777); err != nil {
		panic(err)
	}

	// 4) 下载 mods：通过 CF API 解析下载直链；放置到 ./mods 目录
	if len(manifest.Files) == 0 {
		return
	}

	if os.Getenv("CF_API_KEY") == "" {
		fmt.Println("未设置 CF_API_KEY，跳过 CurseForge 模组下载。已完成 overrides 应用与 inst.json 生成。")
		fmt.Println("如需自动下载 CF 模组，请设置环境变量 CF_API_KEY 后重试。")
		return
	}

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

			// 解析直链并以 URL 最末文件名保存
			url, err := resolveCFDownloadURL(entry.ProjectID, entry.FileID)
			if err != nil {
				errChan <- err
				return
			}
			// 从 URL 提取文件名
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
			fmt.Println("尝试下载:", url)
			if err := core.DownloadFile(url, dst); err != nil {
				errChan <- fmt.Errorf("下载失败(Project %d, File %d): %v", entry.ProjectID, entry.FileID, err)
				return
			}
		}(mf)
	}

	wg.Wait()
	close(errChan)
	for err := range errChan {
		if err != nil {
			panic(err)
		}
	}
}
