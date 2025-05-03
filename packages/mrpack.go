package packages

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/autoinst/AutoInstall/core"
)

type ModrinthIndex struct {
	Files []struct {
		Path      string   `json:"path"`
		Downloads []string `json:"downloads"`
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
	javaPath, simpfun := core.FindJava()
	if javaPath == "" {
		log.Println("未找到 Java，请确保已安装 Java 并设置 PATH。")
		return
	}
	fmt.Println("找到 Java 运行环境:", javaPath)
	if simpfun {
		fmt.Println("已启用 simpfun 环境")
	}
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

	// 安装 NeoForge
	if modrinthIndex.Dependencies.NeoForge != "" {
		fmt.Println("开始安装 NeoForge")
		config := core.InstConfig{
			Version:       modrinthIndex.Dependencies.Minecraft,
			Loader:        "neoforge",
			LoaderVersion: modrinthIndex.Dependencies.NeoForge,
			Download:      "bmclapi",
		}
		NeoForgeB(config, simpfun)
	}
	// 安装 Forge
	if modrinthIndex.Dependencies.Forge != "" {
		fmt.Println("开始安装 Forge")
		config := core.InstConfig{
			Version:       modrinthIndex.Dependencies.Minecraft,
			Loader:        "forge",
			LoaderVersion: modrinthIndex.Dependencies.Forge,
			Download:      "bmclapi",
		}
		ForgeB(config, simpfun)
	}
	// 安装 Fabric
	if modrinthIndex.Dependencies.Fabric != "" {
		fmt.Println("开始安装 Fabric")
		config := core.InstConfig{
			Version:       modrinthIndex.Dependencies.Minecraft,
			Loader:        "fabric",
			LoaderVersion: modrinthIndex.Dependencies.Fabric,
			Download:      "bmclapi",
		}
		FabricB(config, simpfun)
	}

	for _, file := range modrinthIndex.Files {
		filePath := filepath.Join("./", file.Path)
		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			panic(err)
		}
		for _, downloadURL := range file.Downloads {
			err := core.DownloadFile(filePath, downloadURL)
			if err != nil {
				panic(err)
			}
		}
	}
	_ = os.Remove(indexFile.Name())
	os.Exit(0)
}
