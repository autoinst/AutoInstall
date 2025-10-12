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
	err := filepath.Walk(overridesPath, func(path string, info os.FileInfo, walkErr error) error {
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
	idx := "modrinth.index.json"
	if file != "" && strings.HasSuffix(file, ".json") {
		idx = file
	}
	indexPath := filepath.Join("./", idx)
	indexFile, err := os.Open(indexPath)
	if err != nil {
		fmt.Println("未找到modrinth.index.json")
		os.Exit(0)
	}
	defer indexFile.Close()

	byteValue, _ := io.ReadAll(indexFile)

	var modrinthIndex ModrinthIndex
	err = json.Unmarshal(byteValue, &modrinthIndex)
	if err != nil {
		panic(err)
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
		panic(err)
	}

	err = os.WriteFile("inst.json", jsonData, 0777)
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	maxConcurrency := 24

	semaphore := make(chan struct{}, maxConcurrency)
	var errChan = make(chan error, len(modrinthIndex.Files))

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
				fmt.Println("尝试下载:", downloadURL)
				downloadErr = core.DownloadFile(downloadURL, filePath)
				if downloadErr == nil {
					break
				}
				fmt.Printf("下载失败: %v, 尝试下一个链接\n", downloadErr)
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
			panic(err)
		}
	}

	_ = os.Remove(indexFile.Name())
}
