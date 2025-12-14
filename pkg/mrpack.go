package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/autoinst/AutoInstall/core"
)

type ModrinthIndex struct {
	Files []struct {
		Path      string            `json:"path"`
		Env       map[string]string `json:"env"`
		Downloads []string          `json:"downloads"`
	} `json:"files"`
	Dependencies struct {
		Minecraft string `json:"minecraft"`
		NeoForge  string `json:"neoforge"`
		Forge     string `json:"forge"`
		Fabric    string `json:"fabric"`
	} `json:"dependencies"`
}

func Modrinth(file string, MaxCon int, Args string) {
	overridesPath := filepath.Join("./", "overrides")
	if err := moveOverrides(overridesPath); err != nil {
		core.Log("移动 overrides 文件失败:", err)
		return
	}

	idx := "modrinth.index.json"
	if file != "" && strings.HasSuffix(strings.ToLower(file), ".json") {
		idx = file
	}
	indexPath := filepath.Join("./", idx)
	indexFile, err := os.Open(indexPath)
	if err != nil {
		core.Log("未找到 modrinth.index.json")
		return
	}
	defer indexFile.Close()

	byteValue, err := io.ReadAll(indexFile)
	if err != nil {
		core.Log("读取 modrinth.index.json 失败:", err)
		return
	}

	var modrinthIndex ModrinthIndex
	if err := json.Unmarshal(byteValue, &modrinthIndex); err != nil {
		core.Log("解析 modrinth.index.json 失败:", err)
		return
	}
	// 创建 inst.json 文件
	instConfig := core.InstConfig{
		Version:        modrinthIndex.Dependencies.Minecraft,
		Download:       "bmclapi",
		MaxConnections: 32,
		Argsment:       "-Xmx{maxmen}M -Xms{maxmen}M -XX:+AlwaysPreTouch -XX:+DisableExplicitGC -XX:+ParallelRefProcEnabled -XX:+PerfDisableSharedMem -XX:+UnlockExperimentalVMOptions -XX:+UseG1GC -XX:G1HeapRegionSize=8M -XX:G1HeapWastePercent=5 -XX:G1MaxNewSizePercent=40 -XX:G1MixedGCCountTarget=4 -XX:G1MixedGCLiveThresholdPercent=90 -XX:G1NewSizePercent=30 -XX:G1RSetUpdatingPauseTimePercent=5 -XX:G1ReservePercent=20 -XX:InitiatingHeapOccupancyPercent=15 -XX:MaxGCPauseMillis=200 -XX:MaxTenuringThreshold=1 -XX:SurvivorRatio=32 -Dusing.aikars.flags=https://mcflags.emc.gs -Daikars.new.flags=true",
	}

	if modrinthIndex.Dependencies.NeoForge != "" {
		instConfig.Loader = "neoforge"
		instConfig.LoaderVersion = modrinthIndex.Dependencies.NeoForge
	} else if modrinthIndex.Dependencies.Forge != "" {
		instConfig.Loader = "forge"
		instConfig.LoaderVersion = modrinthIndex.Dependencies.Forge
	} else {
		instConfig.Loader = "fabric"
		instConfig.LoaderVersion = modrinthIndex.Dependencies.Fabric
	}

	// 写入 inst.json 文件
	jsonData, err := json.MarshalIndent(instConfig, "", "  ")
	if err != nil {
		core.Log("生成 inst.json 失败:", err)
		return
	}
	if err := os.WriteFile("inst.json", jsonData, 0777); err != nil {
		core.Log("写入 inst.json 失败:", err)
		return
	}

	var wg sync.WaitGroup
	maxConcurrency := 24
	if MaxCon > 0 {
		maxConcurrency = MaxCon
	}
	semaphore := make(chan struct{}, maxConcurrency)
	errChan := make(chan error, len(modrinthIndex.Files))

	for _, file := range modrinthIndex.Files {
		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		go func(file struct {
			Path      string            `json:"path"`
			Env       map[string]string `json:"env"`
			Downloads []string          `json:"downloads"`
		}) {
			defer func() {
				<-semaphore // 释放信号量
				wg.Done()
			}()

			if val, ok := file.Env["server"]; ok && val == "unsupported" {
				return
			}

			filePath := filepath.Join(file.Path)
			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				errChan <- err
				return
			}

			var downloadErr error
			for _, downloadURL := range file.Downloads {
				core.Log("尝试下载:", downloadURL)
				downloadErr = core.DownloadFile(downloadURL, filePath)
				if downloadErr == nil {
					break
				}
				core.Logf("下载失败: %v, 尝试下一个链接\n", downloadErr)
			}
			if downloadErr != nil {
				errChan <- fmt.Errorf("所有下载链接均失败: %v", downloadErr)
				return
			}
		}(file)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			core.Log("下载出错:", err)
		}
	}

	_ = os.Remove(indexFile.Name())
}

func moveOverrides(overridesPath string) error {
	if stat, err := os.Stat(overridesPath); err != nil || !stat.IsDir() {
		return nil
	}
	if err := filepath.Walk(overridesPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
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
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(overridesPath)
}
