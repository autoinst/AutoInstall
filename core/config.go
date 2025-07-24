package core

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"log"
)

// InstConfig 结构体表示 inst.json 的内容
type InstConfig struct {
	Version        string `json:"version"`
	Loader         string `json:"loader"`
	LoaderVersion  string `json:"loaderVersion"`
	Download       string `json:"download"`
	MaxConnections int    `json:"maxconnections"`
	Argsment       string `json:"argsment"`
}

// Config 定义配置文件的结构
type Config struct {
	MaxConnections int `json:"maxconnections"`
}

// Library 定义库的结构
type Library struct {
	Name      string `json:"name"`
	Downloads struct {
		Artifact struct {
			URL  string `json:"url"`
			Path string `json:"path"`
			SHA1 string `json:"sha1"`
		} `json:"artifact"`
	} `json:"downloads"`
}

// VersionInfo 定义 version.json 文件的结构
type VersionInfo struct {
	Libraries []Library `json:"libraries"`
}

// 从 JAR 文件中提取 version.json
func ExtractVersionJson(jarFilePath string) (VersionInfo, error) {
	var versionInfo VersionInfo
	r, err := zip.OpenReader(jarFilePath)
	if err != nil {
		return versionInfo, fmt.Errorf("无法打开 JAR 文件: %v", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name == "version.json" {
			rc, err := f.Open()
			if err != nil {
				log.Printf("警告: 无法打开 version.json 文件: %v", err)
				continue
			}
			defer rc.Close()
			if err := json.NewDecoder(rc).Decode(&versionInfo); err != nil {
				log.Printf("警告: 无法解析 version.json: %v", err)
				continue
			}
			return versionInfo, nil
		}
	}
	for _, f := range r.File {
		if f.Name == "install_profile.json" {
			rc, err := f.Open()
			if err != nil {
				return versionInfo, fmt.Errorf("无法打开 install_profile.json 文件: %v", err)
			}
			defer rc.Close()

			if err := json.NewDecoder(rc).Decode(&versionInfo); err != nil {
				return versionInfo, fmt.Errorf("无法解析 install_profile.json: %v", err)
			}

			return versionInfo, nil
		}
	}

	return versionInfo, fmt.Errorf("没有找到文件")
}
