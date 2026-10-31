package core

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// LibraryKind describes a server prefetch decision, not an installer execution plan.
type LibraryKind string

const (
	LibraryRemote     LibraryKind = "remote"
	LibraryGenerated  LibraryKind = "generated"
	LibraryEmbedded   LibraryKind = "embedded"
	LibraryClientOnly LibraryKind = "client-only"
	LibraryUnknown    LibraryKind = "unknown"
)

// LibraryDeclaration keeps each source's presence evidence even when duplicate libraries merge.
// In particular, ArtifactURLPresent=false differs from a present, explicitly empty URL.
type LibraryDeclaration struct {
	Source             string
	DownloadsPresent   bool
	ArtifactPresent    bool
	ArtifactURLPresent bool
	ArtifactURL        string
}

func (lib *Library) UnmarshalJSON(data []byte) error {
	type plain Library
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*lib = Library(value)
	downloads, ok := raw["downloads"]
	lib.DownloadsPresent = ok
	var d map[string]json.RawMessage
	if len(downloads) > 0 && json.Unmarshal(downloads, &d) == nil {
		artifact, present := d["artifact"]
		lib.ArtifactPresent = present
		var a map[string]json.RawMessage
		if len(artifact) > 0 && json.Unmarshal(artifact, &a) == nil {
			_, lib.ArtifactURLPresent = a["url"]
		}
	}
	return nil
}

// SideValue preserves side-specific installer data. Unsupported shapes stay unresolved.
type SideValue struct {
	Client string `json:"client"`
	Server string `json:"server"`
}

func (value *SideValue) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	_ = json.Unmarshal(raw["client"], &value.Client)
	_ = json.Unmarshal(raw["server"], &value.Server)
	return nil
}

type InstallProcessor struct {
	Sides     []string                   `json:"sides"`
	Jar       string                     `json:"jar"`
	Classpath []string                   `json:"classpath"`
	Args      []string                   `json:"args"`
	Outputs   map[string]json.RawMessage `json:"outputs"`
}

// RequiredServerArtifact paths are relative to the installation root, including libraries/.
// SHA1 may be empty when the installer declares an output but no resolvable hash.
type RequiredServerArtifact struct {
	Path string
	SHA1 string
}

// MavenArtifactPath supports the legacy Maven coordinate, classifier and @extension formats.
func MavenArtifactPath(name string) (string, error) {
	coordinate, extension, hasExtension := strings.Cut(strings.TrimSpace(name), "@")
	if !hasExtension {
		extension = "jar"
	}
	parts := strings.Split(coordinate, ":")
	if len(parts) < 3 || len(parts) > 4 {
		return "", fmt.Errorf("无效 Maven 坐标: %s", name)
	}
	for _, part := range parts {
		if part == "" || strings.ContainsAny(part, "/\\{}[]@") || part == "." || part == ".." {
			return "", fmt.Errorf("无效 Maven 坐标: %s", name)
		}
	}
	if extension == "" || strings.ContainsAny(extension, "/\\:.{}[]@") {
		return "", fmt.Errorf("无效 Maven 扩展名: %s", name)
	}
	file := parts[1] + "-" + parts[2]
	if len(parts) == 4 {
		file += "-" + parts[3]
	}
	return metadataRelativePath(strings.ReplaceAll(parts[0], ".", "/") + "/" + parts[1] + "/" + parts[2] + "/" + file + "." + extension)
}

func LibraryArtifactPath(lib Library) (string, error) {
	if artifact := strings.TrimSpace(lib.Downloads.Artifact.Path); artifact != "" {
		return metadataRelativePath(artifact)
	}
	if file := strings.TrimSpace(lib.FilePath); file != "" {
		parts := strings.Split(lib.Name, ":")
		if len(parts) < 3 {
			return metadataRelativePath(file)
		}
		return metadataRelativePath(strings.ReplaceAll(parts[0], ".", "/") + "/" + parts[1] + "/" + parts[2] + "/" + file)
	}
	return MavenArtifactPath(lib.Name)
}

func metadataRelativePath(value string) (string, error) {
	// Reject Windows paths even when the test or installer runs on Unix.
	if value == "" || strings.ContainsAny(value, "\\:\x00{}[]") || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("不安全的元数据路径: %s", value)
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return "", fmt.Errorf("不安全的元数据路径: %s", value)
		}
	}
	value = path.Clean(value)
	if value == "." {
		return "", fmt.Errorf("空元数据路径")
	}
	return value, nil
}

func knownProcessorSides(p InstallProcessor) bool {
	for _, side := range p.Sides {
		if side != "server" && side != "client" {
			return false
		}
	}
	return true
}

func processorSide(p InstallProcessor, side string) bool {
	if len(p.Sides) == 0 {
		return true
	}
	for _, s := range p.Sides {
		if s == side {
			return true
		}
	}
	return false
}

// resolveData only expands known whole-value references; it never evaluates expressions.
func resolveData(value, side string, data map[string]SideValue, visiting map[string]bool) (string, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") && strings.Count(value, "{") == 1 {
		key := value[1 : len(value)-1]
		pair, ok := data[key]
		if !ok || visiting[key] || len(visiting) >= 32 {
			return "", false
		}
		visiting[key] = true
		defer delete(visiting, key)
		next := pair.Server
		if side == "client" {
			next = pair.Client
		}
		return resolveData(next, side, data, visiting)
	}
	if value == "" {
		return "", false
	}
	return value, true
}

func outputPath(value, side string, data map[string]SideValue) (string, bool) {
	value, ok := resolveData(value, side, data, map[string]bool{})
	if !ok {
		return "", false
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		p, err := MavenArtifactPath(value[1 : len(value)-1])
		return "libraries/" + p, err == nil
	}
	value = strings.TrimPrefix(value, "{ROOT}/")
	if strings.HasPrefix(value, "{LIBRARY_DIR}/") {
		value = "libraries/" + strings.TrimPrefix(value, "{LIBRARY_DIR}/")
	}
	p, err := metadataRelativePath(value)
	return p, err == nil
}

func outputSHA1(value, side string, data map[string]SideValue) (string, bool) {
	value, ok := resolveData(value, side, data, map[string]bool{})
	if !ok {
		return "", false
	}
	value = strings.Trim(value, "'")
	bytes, err := hex.DecodeString(value)
	return strings.ToLower(value), err == nil && len(bytes) == 20
}

func processorReferences(p InstallProcessor, side string, data map[string]SideValue) []string {
	refs := append([]string{p.Jar}, p.Classpath...)
	for _, arg := range p.Args {
		value, ok := resolveData(arg, side, data, map[string]bool{})
		if ok && strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			coordinate := value[1 : len(value)-1]
			if _, err := MavenArtifactPath(coordinate); err == nil {
				refs = append(refs, coordinate)
			}
		}
	}
	return refs
}

func modernLibrary(lib Library) bool {
	return lib.DownloadsPresent || lib.ArtifactPresent || lib.ArtifactURLPresent || lib.Downloads.Artifact.Path != "" || lib.Downloads.Artifact.SHA1 != ""
}

func profileOnlyLibrary(lib Library) bool {
	if len(lib.Sources) == 0 {
		return false
	}
	for _, source := range lib.Sources {
		if source != "install_profile.json" {
			return false
		}
	}
	return true
}

// AnalyzeServerMetadata conservatively classifies prefetch work. All processors still run
// exclusively in the official installer; unresolved metadata is reported, never evaluated.
func AnalyzeServerMetadata(info *VersionInfo) error {
	info.RequiredServerArtifacts = nil
	info.MetadataWarnings = nil
	outputs := map[string]map[string]string{"server": {}, "client": {}}
	processorUsage := map[string]map[string]bool{"server": {}, "client": {}}
	for _, p := range info.Processors {
		knownSides := knownProcessorSides(p)
		for _, side := range []string{"server", "client"} {
			if knownSides && !processorSide(p, side) {
				continue
			}
			refs := processorReferences(p, side, info.Data)
			for _, name := range refs {
				processorUsage[side][name] = true
			}
		}
		if !knownSides {
			info.MetadataWarnings = append(info.MetadataWarnings, "未知处理器 sides，保留其依赖并交由官方安装器处理")
			continue
		}
		for _, side := range []string{"server", "client"} {
			if !processorSide(p, side) {
				continue
			}
			keys := make([]string, 0, len(p.Outputs))
			for key := range p.Outputs {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				target, ok := outputPath(key, side, info.Data)
				if !ok {
					info.MetadataWarnings = append(info.MetadataWarnings, fmt.Sprintf("无法解析 %s 处理器输出 %s，交由官方安装器处理", side, key))
					continue
				}
				var value string
				_ = json.Unmarshal(p.Outputs[key], &value)
				hash, valid := outputSHA1(value, side, info.Data)
				if !valid {
					hash = ""
					info.MetadataWarnings = append(info.MetadataWarnings, fmt.Sprintf("无法解析处理器输出 %s 的 SHA1，安装后仅检查文件存在且非空", target))
				}
				old := outputs[side][target]
				if old != "" && hash != "" && !strings.EqualFold(old, hash) {
					return fmt.Errorf("处理器输出 %s SHA1 冲突", target)
				}
				if hash == "" {
					hash = old
				}
				outputs[side][target] = hash
			}
		}
	}
	embedded := map[string]bool{}
	for _, name := range info.EmbeddedFiles {
		embedded[name] = true
	}
	for i := range info.Libraries {
		lib := &info.Libraries[i]
		lib.Kind, lib.Reason, lib.EmbeddedPath = LibraryUnknown, "", ""
		artifact, err := LibraryArtifactPath(*lib)
		if err != nil {
			if strings.TrimSpace(lib.Downloads.Artifact.URL) != "" || !modernLibrary(*lib) {
				return fmt.Errorf("必需远程库 %s 路径无效: %w", lib.Name, err)
			}
			lib.Reason = err.Error()
			info.MetadataWarnings = append(info.MetadataWarnings, lib.Name+": "+lib.Reason)
			continue
		}
		target := "libraries/" + artifact
		serverHash, serverGenerated := outputs["server"][target]
		_, clientGenerated := outputs["client"][target]
		if serverGenerated {
			if sha := librarySHA1(*lib); sha != "" && serverHash != "" && !strings.EqualFold(sha, serverHash) {
				return fmt.Errorf("库 %s 与处理器输出 SHA1 冲突", lib.Name)
			}
			if serverHash == "" {
				outputs["server"][target] = librarySHA1(*lib)
			}
			lib.Kind, lib.Reason = LibraryGenerated, "服务端处理器声明的输出"
			continue
		}
		if profileOnlyLibrary(*lib) && processorUsage["client"][lib.Name] && !processorUsage["server"][lib.Name] {
			lib.Kind, lib.Reason = LibraryClientOnly, "仅客户端处理器使用且不在运行时版本库列表中"
			continue
		}
		for _, candidate := range []string{"maven/" + artifact, "libraries/" + artifact, artifact, lib.FilePath} {
			if candidate != "" && embedded[candidate] {
				lib.Kind, lib.Reason, lib.EmbeddedPath = LibraryEmbedded, "安装器内嵌文件", candidate
				break
			}
		}
		if lib.Kind == LibraryEmbedded {
			continue
		}
		// Explicit remote libraries are retained even if also used by a client processor.
		if strings.TrimSpace(lib.Downloads.Artifact.URL) != "" {
			lib.Kind, lib.Reason = LibraryRemote, "显式远程 artifact"
			continue
		}
		if clientGenerated {
			if processorUsage["server"][lib.Name] {
				lib.Reason = "客户端生成产物同时被服务端处理器引用，预下载判定不完整，交由官方安装器处理"
				info.MetadataWarnings = append(info.MetadataWarnings, lib.Name+": "+lib.Reason)
			} else {
				lib.Kind, lib.Reason = LibraryClientOnly, "仅客户端处理器声明的输出"
			}
			continue
		}
		if !modernLibrary(*lib) {
			lib.Kind, lib.Reason = LibraryRemote, "旧 Maven 元数据兼容"
		} else {
			lib.Reason = "现代元数据未提供远程 artifact URL，预下载判定不完整，交由官方安装器处理"
			info.MetadataWarnings = append(info.MetadataWarnings, lib.Name+": "+lib.Reason)
		}
	}
	keys := make([]string, 0, len(outputs["server"]))
	for target := range outputs["server"] {
		keys = append(keys, target)
	}
	sort.Strings(keys)
	for _, target := range keys {
		info.RequiredServerArtifacts = append(info.RequiredServerArtifacts, RequiredServerArtifact{Path: target, SHA1: outputs["server"][target]})
	}
	return nil
}

// VerifyRequiredServerArtifacts checks known server processor outputs after installation.
// Unknown expressions remain the official installer's responsibility and emit metadata warnings.
func VerifyRequiredServerArtifacts(root string, info VersionInfo) error {
	for _, artifact := range info.RequiredServerArtifacts {
		relative, err := metadataRelativePath(artifact.Path)
		if err != nil {
			return err
		}
		file := filepath.Join(root, filepath.FromSlash(relative))
		stat, err := os.Stat(file)
		if err != nil {
			return fmt.Errorf("服务端生成产物 %s 不存在或无法读取: %w", file, err)
		}
		if !stat.Mode().IsRegular() || stat.Size() == 0 {
			return fmt.Errorf("服务端生成产物 %s 不是非空普通文件", file)
		}
		if artifact.SHA1 != "" {
			if err := VerifyFileHash(file, "sha1", artifact.SHA1); err != nil {
				return fmt.Errorf("服务端生成产物 %s 校验失败: %w", file, err)
			}
		}
	}
	return nil
}
