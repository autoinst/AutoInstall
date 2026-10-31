package core

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/autoinst/AutoInstall/internal/download"
)

const DefaultMaxRetries = download.DefaultRetries

type InstConfig struct {
	Context        context.Context `json:"-"`
	Version        string          `json:"version"`
	Loader         string          `json:"loader"`
	LoaderVersion  string          `json:"loaderVersion"`
	Download       string          `json:"download"`
	MaxConnections int             `json:"maxconnections"`
	MaxRetries     int             `json:"maxretries"`
	Argsment       string          `json:"argsment"`
}

type Config struct {
	MaxConnections int `json:"maxconnections"`
}

type Library struct {
	// Presence flags distinguish legacy Maven entries from modern entries with no remote artifact.
	DownloadsPresent   bool                 `json:"-"`
	ArtifactPresent    bool                 `json:"-"`
	ArtifactURLPresent bool                 `json:"-"`
	Sources            []string             `json:"-"`
	Declarations       []LibraryDeclaration `json:"-"`
	Kind               LibraryKind          `json:"-"`
	Reason             string               `json:"-"`
	EmbeddedPath       string               `json:"-"`
	Name               string               `json:"name"`
	URL                string               `json:"url"`
	FilePath           string               `json:"filePath"`
	Checksums          []string             `json:"checksums"`
	Downloads          struct {
		Artifact struct {
			URL  string `json:"url"`
			Path string `json:"path"`
			SHA1 string `json:"sha1"`
		} `json:"artifact"`
	} `json:"downloads"`
}

type VersionInfo struct {
	MinecraftVersion        string                   `json:"inheritsFrom"`
	Libraries               []Library                `json:"libraries"`
	ServerJarPath           string                   `json:"serverJarPath"`
	InstallFilePath         string                   `json:"installFilePath"`
	Data                    map[string]SideValue     `json:"data"`
	Processors              []InstallProcessor       `json:"processors"`
	EmbeddedFiles           []string                 `json:"-"`
	RequiredServerArtifacts []RequiredServerArtifact `json:"-"`
	MetadataWarnings        []string                 `json:"-"`
}

type installProfile struct {
	MinecraftVersion string      `json:"minecraft"`
	Libraries        []Library   `json:"libraries"`
	VersionInfo      VersionInfo `json:"versionInfo"`
	Install          struct {
		Path     string `json:"path"`
		FilePath string `json:"filePath"`
	} `json:"install"`
	ServerJarPath string               `json:"serverJarPath"`
	Data          map[string]SideValue `json:"data"`
	Processors    []InstallProcessor   `json:"processors"`
}

func NormalizeRetries(retries int) int { return download.NormalizeRetries(retries) }

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
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			merged.EmbeddedFiles = append(merged.EmbeddedFiles, f.Name)
		}
	}
	if err := AnalyzeServerMetadata(&merged); err != nil {
		return merged, err
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
		for i := range info.Libraries {
			lib := &info.Libraries[i]
			source := name
			if len(lib.Sources) > 0 {
				source = lib.Sources[0]
			}
			lib.Declarations = append(lib.Declarations, LibraryDeclaration{Source: source, DownloadsPresent: lib.DownloadsPresent, ArtifactPresent: lib.ArtifactPresent, ArtifactURLPresent: lib.ArtifactURLPresent, ArtifactURL: lib.Downloads.Artifact.URL})
			lib.Sources = append(lib.Sources, name)
		}
		return info, true, nil
	}
	return VersionInfo{}, false, nil
}

func (profile installProfile) toVersionInfo() VersionInfo {
	info := profile.VersionInfo
	if profile.MinecraftVersion != "" {
		info.MinecraftVersion = profile.MinecraftVersion
	}
	// Both lists may carry useful entries; merging later also enforces SHA1 conflicts.
	for i := range info.Libraries {
		info.Libraries[i].Sources = append(info.Libraries[i].Sources, "install_profile.json/versionInfo")
	}
	info.Libraries = append(info.Libraries, profile.Libraries...)
	if info.Data == nil {
		info.Data = make(map[string]SideValue)
	}
	for key, value := range profile.Data {
		info.Data[key] = value
	}
	info.Processors = append(info.Processors, profile.Processors...)
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
	if src.MinecraftVersion != "" {
		if dst.MinecraftVersion != "" && src.MinecraftVersion != dst.MinecraftVersion {
			return fmt.Errorf("安装器 Minecraft 版本元数据冲突: %s 与 %s", dst.MinecraftVersion, src.MinecraftVersion)
		}
		dst.MinecraftVersion = src.MinecraftVersion
	}
	if dst.Data == nil {
		dst.Data = make(map[string]SideValue)
	}
	for key, value := range src.Data {
		dst.Data[key] = value
	}
	dst.Processors = append(dst.Processors, src.Processors...)
	dst.EmbeddedFiles = append(dst.EmbeddedFiles, src.EmbeddedFiles...)
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
	if path, err := LibraryArtifactPath(lib); err == nil {
		return path
	}
	if url := strings.TrimSpace(lib.Downloads.Artifact.URL); url != "" {
		return url
	}
	return strings.TrimSpace(lib.Name)
}

func mergeLibrary(dst, src Library) Library {
	dst.DownloadsPresent = dst.DownloadsPresent || src.DownloadsPresent
	dst.ArtifactPresent = dst.ArtifactPresent || src.ArtifactPresent
	dst.ArtifactURLPresent = dst.ArtifactURLPresent || src.ArtifactURLPresent
	dst.Sources = append(dst.Sources, src.Sources...)
	dst.Declarations = append(dst.Declarations, src.Declarations...)
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
