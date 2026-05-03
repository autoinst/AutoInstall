package pkg

import (
	"encoding/json"
	"errors"
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
		Hashes    map[string]string `json:"hashes"`
	} `json:"files"`
	Dependencies ModrinthDependencies `json:"dependencies"`
}

type ModrinthDependencies struct {
	Minecraft    string
	NeoForge     string
	Forge        string
	Fabric       string
	FabricLoader string
}

func (d *ModrinthDependencies) UnmarshalJSON(data []byte) error {
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	d.Minecraft = raw["minecraft"]
	d.NeoForge = raw["neoforge"]
	d.Forge = raw["forge"]
	d.Fabric = raw["fabric"]
	d.FabricLoader = raw["fabric-loader"]
	return nil
}

func Modrinth(file string, MaxCon int, Args string, bundleName string, MaxRetries int, baseConfig core.InstConfig) error {
	MaxRetries = core.NormalizeRetries(MaxRetries)
	overridesPath := filepath.Join("./", "overrides")
	if err := moveOverrides(overridesPath); err != nil {
		return fmt.Errorf("移动 overrides 文件失败: %w", err)
	}

	idx := "modrinth.index.json"
	if file != "" && strings.HasSuffix(strings.ToLower(file), ".json") {
		idx = file
	}
	indexPath := filepath.Join("./", idx)
	indexFile, err := os.Open(indexPath)
	if err != nil {
		return fmt.Errorf("未找到 modrinth.index.json: %w", err)
	}
	defer indexFile.Close()

	byteValue, err := io.ReadAll(indexFile)
	if err != nil {
		return fmt.Errorf("读取 modrinth.index.json 失败: %w", err)
	}

	var modrinthIndex ModrinthIndex
	if err := json.Unmarshal(byteValue, &modrinthIndex); err != nil {
		return fmt.Errorf("解析 modrinth.index.json 失败: %w", err)
	}
	minecraftVersion := modrinthIndex.Dependencies.Minecraft
	loaderVersion := modrinthIndex.Dependencies.NeoForge
	loaderName := ""
	switch {
	case modrinthIndex.Dependencies.NeoForge != "":
		loaderName = "neoforge"
		loaderVersion = modrinthIndex.Dependencies.NeoForge
	case modrinthIndex.Dependencies.Forge != "":
		loaderName = "forge"
		loaderVersion = modrinthIndex.Dependencies.Forge
	case modrinthIndex.Dependencies.FabricLoader != "":
		loaderName = "fabric"
		loaderVersion = modrinthIndex.Dependencies.FabricLoader
	case modrinthIndex.Dependencies.Fabric != "":
		loaderName = "fabric"
		loaderVersion = modrinthIndex.Dependencies.Fabric
	}

	instConfig := modpackBaseConfig(baseConfig, MaxRetries)
	instConfig.Version = minecraftVersion

	if loaderName != "" {
		instConfig.Loader = loaderName
		instConfig.LoaderVersion = loaderVersion
	} else {
		core.Log("未找到可识别的加载器依赖，inst.json 将只写入 minecraft 版本")
	}

	jsonData, err := json.MarshalIndent(instConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("生成 inst.json 失败: %w", err)
	}
	if err := os.WriteFile("inst.json", jsonData, 0777); err != nil {
		return fmt.Errorf("写入 inst.json 失败: %w", err)
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
		semaphore <- struct{}{}

		go func(file struct {
			Path      string            `json:"path"`
			Env       map[string]string `json:"env"`
			Downloads []string          `json:"downloads"`
			Hashes    map[string]string `json:"hashes"`
		}) {
			defer func() {
				<-semaphore
				wg.Done()
			}()

			if val, ok := file.Env["server"]; ok && strings.EqualFold(strings.TrimSpace(val), "unsupported") {
				return
			}
			if len(file.Downloads) == 0 {
				errChan <- fmt.Errorf("文件 %s 没有下载链接", file.Path)
				return
			}

			filePath, err := safeJoin(".", file.Path)
			if err != nil {
				errChan <- err
				return
			}
			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				errChan <- err
				return
			}

			var downloadErr error
			for _, downloadURL := range file.Downloads {
				core.Log("尝试下载:", downloadURL)
				downloadErr = core.DownloadFileRetry(downloadURL, filePath, MaxRetries)
				if downloadErr == nil {
					break
				}
				core.Logf("下载失败: %v, 尝试下一个链接\n", downloadErr)
			}
			if downloadErr != nil {
				errChan <- fmt.Errorf("所有下载链接均失败: %v", downloadErr)
				return
			}
			if err := verifyModrinthFile(filePath, file.Hashes); err != nil {
				_ = os.Remove(filePath)
				errChan <- fmt.Errorf("文件 %s 校验失败: %w", file.Path, err)
				return
			}
		}(file)
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
	_ = os.Remove(indexFile.Name())
	return nil
}

func verifyModrinthFile(filePath string, hashes map[string]string) error {
	if hashes == nil {
		return nil
	}
	if sha512 := hashes["sha512"]; sha512 != "" {
		return core.VerifyFileHash(filePath, "sha512", sha512)
	}
	if sha1 := hashes["sha1"]; sha1 != "" {
		return core.VerifyFileHash(filePath, "sha1", sha1)
	}
	return nil
}

func moveOverrides(overridesPath string) error {
	if stat, err := os.Stat(overridesPath); err != nil || !stat.IsDir() {
		return nil
	}

	type moveItem struct {
		src string
		dst string
	}
	var items []moveItem
	var conflicts []string

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
		destPath, err := safeJoin(".", relPath)
		if err != nil {
			return err
		}
		if _, err := os.Stat(destPath); err == nil {
			same, err := sameFileContent(path, destPath)
			if err != nil {
				return err
			}
			if !same {
				conflicts = append(conflicts, relPath)
			}
		}
		items = append(items, moveItem{src: path, dst: destPath})
		return nil
	}); err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("overrides 文件冲突，未覆盖现有文件: %s", strings.Join(conflicts, ", "))
	}

	for _, item := range items {
		if err := os.MkdirAll(filepath.Dir(item.dst), os.ModePerm); err != nil {
			return err
		}
		if same, _ := sameFileContent(item.src, item.dst); same {
			_ = os.Remove(item.src)
			continue
		}
		if err := os.Rename(item.src, item.dst); err != nil {
			return err
		}
	}
	return os.RemoveAll(overridesPath)
}

func sameFileContent(a, b string) (bool, error) {
	left, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	right, err := os.ReadFile(b)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return string(left) == string(right), nil
}
