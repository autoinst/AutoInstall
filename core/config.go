package core

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"strings"
)

const DefaultMaxRetries = 3

type InstConfig struct {
	Version        string `json:"version"`
	Loader         string `json:"loader"`
	LoaderVersion  string `json:"loaderVersion"`
	Download       string `json:"download"`
	MaxConnections int    `json:"maxconnections"`
	MaxRetries     int    `json:"maxretries"`
	Argsment       string `json:"argsment"`
}

type Config struct {
	MaxConnections int `json:"maxconnections"`
}

type Library struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	FilePath  string   `json:"filePath"`
	Checksums []string `json:"checksums"`
	Downloads struct {
		Artifact struct {
			URL  string `json:"url"`
			Path string `json:"path"`
			SHA1 string `json:"sha1"`
		} `json:"artifact"`
	} `json:"downloads"`
}

type VersionInfo struct {
	Libraries       []Library `json:"libraries"`
	ServerJarPath   string    `json:"serverJarPath"`
	InstallFilePath string    `json:"installFilePath"`
}

type installProfile struct {
	Libraries   []Library   `json:"libraries"`
	VersionInfo VersionInfo `json:"versionInfo"`
	Install     struct {
		Path     string `json:"path"`
		FilePath string `json:"filePath"`
	} `json:"install"`
	ServerJarPath string `json:"serverJarPath"`
}

func NormalizeRetries(retries int) int {
	if retries <= 0 {
		return DefaultMaxRetries
	}
	return retries
}

func ApplyConfigDefaults(config *InstConfig) {
	config.MaxRetries = NormalizeRetries(config.MaxRetries)
}

func ExtractVersionJson(jarFilePath string) (VersionInfo, error) {
	var merged VersionInfo
	r, err := zip.OpenReader(jarFilePath)
	if err != nil {
		return merged, fmt.Errorf("无法打开 JAR 文件: %v", err)
	}
	defer r.Close()

	found := false
	for _, name := range []string{"version.json", "install_profile.json"} {
		info, ok, err := decodeVersionInfoFromZipFile(r.File, name)
		if err != nil {
			return merged, err
		}
		if !ok {
			continue
		}
		found = true
		if err := mergeVersionInfo(&merged, info); err != nil {
			return merged, fmt.Errorf("合并 %s 失败: %w", name, err)
		}
	}

	if !found {
		return merged, fmt.Errorf("没有找到 version.json 或 install_profile.json")
	}
	return merged, nil
}

func decodeVersionInfoFromZipFile(files []*zip.File, name string) (VersionInfo, bool, error) {
	for _, f := range files {
		if f.Name != name {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return VersionInfo{}, true, fmt.Errorf("无法打开 %s 文件: %w", name, err)
		}
		defer rc.Close()

		var info VersionInfo
		if name == "install_profile.json" {
			var profile installProfile
			if err := json.NewDecoder(rc).Decode(&profile); err != nil {
				return VersionInfo{}, true, fmt.Errorf("无法解析 %s: %w", name, err)
			}
			info = profile.toVersionInfo()
		} else if err := json.NewDecoder(rc).Decode(&info); err != nil {
			return VersionInfo{}, true, fmt.Errorf("无法解析 %s: %w", name, err)
		}
		return info, true, nil
	}
	return VersionInfo{}, false, nil
}

func (profile installProfile) toVersionInfo() VersionInfo {
	info := profile.VersionInfo
	if len(info.Libraries) == 0 {
		info.Libraries = profile.Libraries
	}
	if strings.TrimSpace(info.ServerJarPath) == "" {
		info.ServerJarPath = profile.ServerJarPath
	}
	if strings.TrimSpace(info.InstallFilePath) == "" {
		info.InstallFilePath = profile.Install.FilePath
	}
	if strings.TrimSpace(profile.Install.Path) != "" {
		installLib := Library{
			Name:     profile.Install.Path,
			FilePath: profile.Install.FilePath,
		}
		info.Libraries = append([]Library{installLib}, info.Libraries...)
	}
	return info
}

func mergeVersionInfo(dst *VersionInfo, src VersionInfo) error {
	if strings.TrimSpace(src.ServerJarPath) != "" {
		dst.ServerJarPath = src.ServerJarPath
	}
	if strings.TrimSpace(src.InstallFilePath) != "" {
		dst.InstallFilePath = src.InstallFilePath
	}

	seen := make(map[string]int, len(dst.Libraries)+len(src.Libraries))
	for i, lib := range dst.Libraries {
		key := libraryKey(lib)
		if key != "" {
			seen[key] = i
		}
	}

	for _, lib := range src.Libraries {
		key := libraryKey(lib)
		if key == "" {
			dst.Libraries = append(dst.Libraries, lib)
			continue
		}

		if existingIndex, ok := seen[key]; ok {
			if hasConflictingSHA1(dst.Libraries[existingIndex], lib) {
				return fmt.Errorf("库 %s 与 %s 使用相同位置 %s 但 SHA1 不一致", lib.Name, dst.Libraries[existingIndex].Name, key)
			}
			dst.Libraries[existingIndex] = mergeLibrary(dst.Libraries[existingIndex], lib)
			continue
		}

		dst.Libraries = append(dst.Libraries, lib)
		seen[key] = len(dst.Libraries) - 1
	}
	return nil
}

func libraryKey(lib Library) string {
	if path := strings.TrimSpace(lib.Downloads.Artifact.Path); path != "" {
		return path
	}
	if url := strings.TrimSpace(lib.Downloads.Artifact.URL); url != "" {
		return url
	}
	return strings.TrimSpace(lib.Name)
}

func mergeLibrary(dst, src Library) Library {
	if strings.TrimSpace(dst.Name) == "" {
		dst.Name = src.Name
	}
	if strings.TrimSpace(dst.URL) == "" {
		dst.URL = src.URL
	}
	if strings.TrimSpace(dst.FilePath) == "" {
		dst.FilePath = src.FilePath
	}
	if len(dst.Checksums) == 0 {
		dst.Checksums = src.Checksums
	}
	if strings.TrimSpace(dst.Downloads.Artifact.URL) == "" {
		dst.Downloads.Artifact.URL = src.Downloads.Artifact.URL
	}
	if strings.TrimSpace(dst.Downloads.Artifact.Path) == "" {
		dst.Downloads.Artifact.Path = src.Downloads.Artifact.Path
	}
	if strings.TrimSpace(dst.Downloads.Artifact.SHA1) == "" {
		dst.Downloads.Artifact.SHA1 = src.Downloads.Artifact.SHA1
	}
	return dst
}

func hasConflictingSHA1(a, b Library) bool {
	aSHA := librarySHA1(a)
	bSHA := librarySHA1(b)
	return aSHA != "" && bSHA != "" && !strings.EqualFold(aSHA, bSHA)
}

func librarySHA1(lib Library) string {
	if sha1 := strings.TrimSpace(lib.Downloads.Artifact.SHA1); sha1 != "" {
		return sha1
	}
	if len(lib.Checksums) > 0 {
		return strings.TrimSpace(lib.Checksums[0])
	}
	return ""
}
