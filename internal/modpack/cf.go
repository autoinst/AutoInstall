package modpack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/autoinst/AutoInstall/internal/download"
	"github.com/autoinst/AutoInstall/internal/model"
	"github.com/autoinst/AutoInstall/internal/workspace"
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

type CFError struct {
	StatusCode int
	Body       string
}

func (e *CFError) Error() string { return fmt.Sprintf("响应异常: %d %s", e.StatusCode, e.Body) }

func resolveCFDownloadURLContext(ctx context.Context, projectID, fileID, maxRetries int, deps Dependencies) (string, error) {
	if deps.CFAPIKey == "" {
		return "", fmt.Errorf("缺少 CF_API_KEY")
	}
	endpoint := fmt.Sprintf("https://api.curseforge.com/v1/mods/%d/files/%d/download-url", projectID, fileID)
	var lastErr error
	for i := 0; i < download.NormalizeRetries(maxRetries); i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("x-api-key", deps.CFAPIKey)
		req.Header.Set("User-Agent", "autoinst/1.4")
		client := deps.Download.HTTP
		if client == nil {
			client = http.DefaultClient
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			lastErr = &CFError{StatusCode: resp.StatusCode, Body: string(b)}
			continue
		}
		var out struct {
			Data string `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if out.Data == "" {
			lastErr = fmt.Errorf("CF API 未返回下载地址")
			continue
		}
		return out.Data, nil
	}
	return "", fmt.Errorf("多次获取 CurseForge 直链失败: %w", lastErr)
}

func curseForgeDownloadFilename(raw string, fileID int) string {
	name, err := workspace.Filename(raw)
	if err != nil {
		return fmt.Sprintf("%d.jar", fileID)
	}
	return name
}

func curseForgeTarget(manifest CurseForgeManifest) (model.Target, error) {
	t := model.Target{Version: strings.TrimSpace(manifest.Minecraft.Version), Loader: "vanilla"}
	id := ""
	for _, loader := range manifest.Minecraft.ModLoaders {
		if id == "" || loader.Primary {
			id = loader.ID
		}
		if loader.Primary {
			break
		}
	}
	if id == "" {
		return t, nil
	}
	for _, name := range []string{"neoforge", "forge", "fabric"} {
		if strings.HasPrefix(strings.ToLower(id), name+"-") {
			t.Loader = name
			t.LoaderVersion = id[len(name)+1:]
			return t, nil
		}
	}
	return t, fmt.Errorf("不支持的整合包加载器: %s", id)
}

func planCurseForge(ctx context.Context, manifest CurseForgeManifest, opts model.Options, deps Dependencies) ([]model.FileTask, error) {
	files := make([]model.FileTask, len(manifest.Files))
	var tasks []func() error
	for i, entry := range manifest.Files {
		if !entry.Required {
			continue
		}
		if entry.ProjectID <= 0 || entry.FileID <= 0 {
			return nil, fmt.Errorf("CurseForge 文件 ID 必须为正整数")
		}
		i, entry := i, entry
		tasks = append(tasks, func() error {
			raw, err := resolveCFDownloadURLContext(ctx, entry.ProjectID, entry.FileID, opts.MaxRetries, deps)
			if err != nil {
				return fmt.Errorf("获取直链失败(Project %d, File %d): %w", entry.ProjectID, entry.FileID, err)
			}
			files[i] = model.FileTask{Path: filepath.Join("mods", curseForgeDownloadFilename(raw, entry.FileID)), URLs: []string{raw}}
			return nil
		})
	}
	if err := runPackTasks(ctx, opts.MaxConnections, tasks); err != nil {
		return nil, err
	}
	result := make([]model.FileTask, 0, len(tasks))
	for _, file := range files {
		if file.Path != "" {
			result = append(result, file)
		}
	}
	return result, nil
}
