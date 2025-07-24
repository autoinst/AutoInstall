package pkg

import (
	"archive/zip"
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

func Modrinth(file string) {
	if strings.HasSuffix(file, ".mrpack") {
		zipFile, err := zip.OpenReader(file)
		if err != nil {
			panic(err)
		}
		defer zipFile.Close()
		for _, f := range zipFile.File {
			filePath := filepath.Join("./", f.Name)
			if f.FileInfo().IsDir() {
				_ = os.MkdirAll(filePath, os.ModePerm)
				continue
			}
			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				panic(err)
			}
			dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				panic(err)
			}
			file, err := f.Open()
			if err != nil {
				panic(err)
			}
			if _, err := io.Copy(dstFile, file); err != nil {
				panic(err)
			}
			dstFile.Close()
			file.Close()
		}
	}
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
	indexPath := filepath.Join("./", "modrinth.index.json")
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
	var config core.InstConfig
	// 创建 inst.json 文件
	instConfig := core.InstConfig{
		Version:        modrinthIndex.Dependencies.Minecraft,
		Download:       "bmclapi",
		MaxConnections: config.MaxConnections,
		Argsment:       config.Argsment,
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
			for _, downloadURL := range file.Downloads {
				fmt.Println("下载链接:", downloadURL)
				err := core.DownloadFile(downloadURL, filePath)
				if err != nil {
					errChan <- err
					return
				}
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
