package modcheck

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/autoinst/AutoInstall/core"
)

func FilterInstalledMods(modsDir, bundleName string, cfg *Config) ([]string, error) {
	config := DefaultConfig
	if cfg != nil {
		config = *cfg
	}

	mods, err := ExtractMods(modsDir)
	if err != nil {
		return nil, err
	}
	if len(mods) == 0 {
		core.Log("未发现可检查的模组，跳过客户端模组检测")
		return nil, nil
	}

	core.Logf("开始检测客户端模组，共 %d 个文件\n", len(mods))
	client := &http.Client{Timeout: config.Timeout}

	var detected []string
	reasons := make(map[string][]string)
	processed := make(map[string]struct{})

	if config.EnableModrinth {
		remaining := excludeProcessed(mods, processed)
		modsByProject := filterByModrinthProject(client, remaining)
		detected = append(detected, modsByProject...)
		recordReasons(reasons, modsByProject, "Modrinth项目")
		for _, filePath := range modsByProject {
			processed[filePath] = struct{}{}
		}
	}

	if config.EnableMixin {
		remaining := excludeProcessed(mods, processed)
		modsByMixin := filterByMixin(remaining)
		detected = append(detected, modsByMixin...)
		recordReasons(reasons, modsByMixin, "Mixin")
		for _, filePath := range modsByMixin {
			processed[filePath] = struct{}{}
		}
	}

	if config.EnableHash {
		remaining := excludeProcessed(mods, processed)
		modsByHash := filterByModrinthHash(client, remaining)
		detected = append(detected, modsByHash...)
		recordReasons(reasons, modsByHash, "Modrinth哈希")
	}

	unique := uniquePaths(detected)
	if len(unique) == 0 {
		core.Log("未识别到客户端模组")
		return nil, nil
	}

	logDetectedMods(unique, reasons)

	targetDir := filepath.Join(".rubbish", normalizeBundleName(bundleName))
	moved, err := moveMods(unique, targetDir)
	if err != nil {
		return moved, err
	}

	core.Logf("已识别并移走 %d 个客户端模组\n", len(moved))
	return moved, nil
}

func excludeProcessed(files []ModFile, processed map[string]struct{}) []ModFile {
	if len(processed) == 0 {
		return files
	}
	out := make([]ModFile, 0, len(files))
	for _, file := range files {
		if _, ok := processed[file.Path]; ok {
			continue
		}
		out = append(out, file)
	}
	return out
}

func uniquePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, filePath := range paths {
		if filePath == "" {
			continue
		}
		if _, ok := seen[filePath]; ok {
			continue
		}
		seen[filePath] = struct{}{}
		out = append(out, filePath)
	}
	return out
}

func recordReasons(reasons map[string][]string, paths []string, reason string) {
	if len(paths) == 0 {
		return
	}
	for _, filePath := range paths {
		reasons[filePath] = appendUniqueReason(reasons[filePath], reason)
	}
}

func appendUniqueReason(existing []string, reason string) []string {
	for _, item := range existing {
		if item == reason {
			return existing
		}
	}
	return append(existing, reason)
}

func logDetectedMods(paths []string, reasons map[string][]string) {
	for _, filePath := range paths {
		hitReasons := reasons[filePath]
		reasonText := "未知来源"
		if len(hitReasons) > 0 {
			reasonText = strings.Join(hitReasons, ", ")
		}
		core.Logf("识别到客户端模组: %s (命中: %s)\n", filepath.Base(filePath), reasonText)
	}
}

func normalizeBundleName(bundleName string) string {
	bundleName = strings.TrimSpace(bundleName)
	if bundleName == "" {
		return "modpack"
	}
	return bundleName
}

func moveMods(paths []string, targetDir string) ([]string, error) {
	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		return nil, err
	}

	moved := make([]string, 0, len(paths))
	var moveErrs []string
	for _, sourcePath := range paths {
		targetPath := filepath.Join(targetDir, filepath.Base(sourcePath))
		if err := moveFile(sourcePath, targetPath); err != nil {
			moveErrs = append(moveErrs, fmt.Sprintf("%s: %v", filepath.Base(sourcePath), err))
			continue
		}
		moved = append(moved, sourcePath)
	}

	if len(moveErrs) > 0 {
		return moved, fmt.Errorf("部分模组移动失败: %s", strings.Join(moveErrs, "; "))
	}
	return moved, nil
}

func moveFile(sourcePath, targetPath string) error {
	if err := os.Rename(sourcePath, targetPath); err == nil {
		return nil
	}

	src, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return os.Remove(sourcePath)
}
