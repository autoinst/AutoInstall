package pkg

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/autoinst/AutoInstall/core"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
)

var DownloadWg sync.WaitGroup

var downloadErrs struct {
	sync.Mutex
	errs []error
}

func WaitDownloads() error {
	DownloadWg.Wait()
	downloadErrs.Lock()
	defer downloadErrs.Unlock()
	return errors.Join(downloadErrs.errs...)
}

func recordAsyncDownloadError(err error) {
	if err == nil {
		return
	}
	downloadErrs.Lock()
	defer downloadErrs.Unlock()
	downloadErrs.errs = append(downloadErrs.errs, err)
}

func Search(MaxConnections int, Argsment string, MaxRetries int, baseConfig core.InstConfig) (bool, error) {
	core.Log("正在扫描可用的整合包...")
	pack, packType, err := detectPackFile()
	if err != nil {
		core.Log(err.Error())
		return false, nil
	}

	if err := installByType(packType, pack, MaxConnections, Argsment, MaxRetries, baseConfig); err != nil {
		return true, fmt.Errorf("安装失败: %w", err)
	}
	return true, nil
}

func detectPackFile() (path string, packType string, err error) {
	mrpackFiles, _ := filepath.Glob("*.mrpack")
	zipFiles, _ := filepath.Glob("*.zip")
	allPacks := append(append([]string{}, mrpackFiles...), zipFiles...)

	if len(allPacks) == 0 {
		if _, err := os.Stat("variables.txt"); err == nil {
			return "variables.txt", "spc-plain", nil
		}
		if _, err := os.Stat("modrinth.index.json"); err == nil {
			return "modrinth.index.json", "modrinth-plain", nil
		}
		if _, err := os.Stat("manifest.json"); err == nil {
			if manifestIsSPCFromFile("manifest.json") {
				return "manifest.json", "spc-plain", nil
			}
			return "manifest.json", "curseforge-plain", nil
		}
		return "", "", errors.New("未找到整合包")
	}

	if len(allPacks) == 1 {
		file := allPacks[0]
		return file, packTypeFromExt(file), nil
	}

	for _, f := range allPacks {
		if strings.EqualFold(f, "modpack.mrpack") {
			return f, packTypeFromExt(f), nil
		}
	}
	for _, f := range allPacks {
		if strings.EqualFold(f, "modpack.zip") {
			return f, packTypeFromExt(f), nil
		}
	}

	var builder strings.Builder
	builder.WriteString("发现多个整合包，但未找到 modpack.zip 或 modpack.mrpack\n")
	builder.WriteString("请将要使用的整合包重命名为 modpack.zip 或 modpack.mrpack 后重试\n")
	for _, file := range allPacks {
		builder.WriteString("  " + file + "\n")
	}
	return "", "", errors.New(builder.String())
}

func packTypeFromExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mrpack":
		return "modrinth"
	case ".zip":
		return "zip"
	default:
		return "unknown"
	}
}

func installByType(packType, path string, MaxConnections int, Argsment string, MaxRetries int, baseConfig core.InstConfig) error {
	bundleName := bundleNameFromPath(path)
	switch packType {
	case "spc-plain":
		return SPCInstall(path, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
	case "modrinth-plain":
		return Modrinth(path, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
	case "curseforge-plain":
		return CurseForge(path, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
	case "modrinth":
		return installModrinthArchive(path, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
	case "curseforge-zip":
		return installCurseForgeArchive(path, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
	case "zip":
		return installZipArchive(path, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
	default:
		return fmt.Errorf("无法识别的整合包类型: %s", packType)
	}
}

func modpackBaseConfig(baseConfig core.InstConfig, maxRetries int) core.InstConfig {
	core.ApplyConfigDefaults(&baseConfig)
	if baseConfig.Download == "" {
		baseConfig.Download = "bmclapi"
	}
	if baseConfig.MaxConnections <= 0 {
		baseConfig.MaxConnections = 32
	}
	baseConfig.MaxRetries = core.NormalizeRetries(maxRetries)
	if strings.TrimSpace(baseConfig.Argsment) == "" {
		baseConfig.Argsment = "-Xmx{maxmen}M -Xms{maxmen}M -XX:+AlwaysPreTouch -XX:+DisableExplicitGC -XX:+ParallelRefProcEnabled -XX:+PerfDisableSharedMem -XX:+UnlockExperimentalVMOptions -XX:+UseG1GC -XX:G1HeapRegionSize=8M -XX:G1HeapWastePercent=5 -XX:G1MaxNewSizePercent=40 -XX:G1MixedGCCountTarget=4 -XX:G1MixedGCLiveThresholdPercent=90 -XX:G1NewSizePercent=30 -XX:G1RSetUpdatingPauseTimePercent=5 -XX:G1ReservePercent=20 -XX:InitiatingHeapOccupancyPercent=15 -XX:MaxGCPauseMillis=200 -XX:MaxTenuringThreshold=1 -XX:SurvivorRatio=32 -Dusing.aikars.flags=https://mcflags.emc.gs -Daikars.new.flags=true"
	}
	return baseConfig
}

func bundleNameFromPath(path string) string {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return "modpack"
	}
	base := filepath.Base(cleanPath)
	if strings.EqualFold(base, "modrinth.index.json") || strings.EqualFold(base, "manifest.json") || strings.EqualFold(base, "variables.txt") {
		wd, err := os.Getwd()
		if err == nil {
			base = filepath.Base(wd)
		}
	}
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	if base == "" || base == "." {
		return "modpack"
	}
	return base
}

// installZipArchive 尝试识别 zip 是 CurseForge 还是 SPC 格式
func installZipArchive(file string, MaxConnections int, Argsment string, bundleName string, MaxRetries int, baseConfig core.InstConfig) error {
	if zipContains(file, "modrinth.index.json") {
		return installModrinthArchive(file, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
	}
	if zipContains(file, "manifest.json") {
		if manifestIsSPCFromZip(file) {
			return installSPCArchive(file, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
		}
		return installCurseForgeArchive(file, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
	}
	return installSPCArchive(file, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
}

func manifestIsSPCFromFile(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return false
	}
	_, ok := m["serverPackCreatorVersion"]
	return ok
}

func manifestIsSPCFromZip(zipPath string) bool {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return false
	}
	defer r.Close()
	for _, f := range r.File {
		name := f.Name
		if f.NonUTF8 {
			if fixed, ok := tryFixZipName(name); ok {
				name = fixed
			}
		}
		if filepath.Base(name) == "manifest.json" {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			b, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				continue
			}
			var m map[string]interface{}
			if err := json.Unmarshal(b, &m); err == nil {
				if _, ok := m["serverPackCreatorVersion"]; ok {
					return true
				}
			}
		}
	}
	return false
}

func installCurseForgeArchive(file string, MaxConnections int, Argsment string, bundleName string, MaxRetries int, baseConfig core.InstConfig) error {
	manifestEntry, err := findZipEntry(file, "manifest.json")
	if err != nil {
		return err
	}
	if err := extractZip(file, nil); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}
	manifestPath, err := safeJoin(".", manifestEntry)
	if err != nil {
		return err
	}
	return CurseForge(manifestPath, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
}

func installSPCArchive(file string, MaxConnections int, Argsment string, bundleName string, MaxRetries int, baseConfig core.InstConfig) error {
	variablesEntry, err := findZipEntry(file, "variables.txt")
	if err != nil {
		return err
	}
	skipScripts := map[string]struct{}{"start.sh": {}, "run.sh": {}}
	if err := extractZip(file, skipScripts); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}
	variablesPath, err := safeJoin(".", variablesEntry)
	if err != nil {
		return err
	}
	return SPCInstall(variablesPath, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
}

func installModrinthArchive(file string, MaxConnections int, Argsment string, bundleName string, MaxRetries int, baseConfig core.InstConfig) error {
	indexEntry, err := findZipEntry(file, "modrinth.index.json")
	if err != nil {
		return err
	}
	if err := extractZip(file, nil); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}
	indexPath, err := safeJoin(".", indexEntry)
	if err != nil {
		return err
	}
	return Modrinth(indexPath, MaxConnections, Argsment, bundleName, MaxRetries, baseConfig)
}

func zipContains(zipPath, target string) bool {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return false
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name == target {
			return true
		}
		if f.NonUTF8 {
			if fixed, ok := tryFixZipName(f.Name); ok && fixed == target {
				return true
			}
		}
		if filepath.Base(f.Name) == target {
			return true
		}
		if f.NonUTF8 {
			if fixed, ok := tryFixZipName(f.Name); ok && filepath.Base(fixed) == target {
				return true
			}
		}
	}
	return false
}

func extractZip(archivePath string, skipNames map[string]struct{}) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := zipEntryName(f)
		if skipNames != nil {
			if _, ok := skipNames[name]; ok {
				continue
			}
			if _, ok := skipNames[filepath.Base(name)]; ok {
				continue
			}
		}
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("不支持解压符号链接: %s", name)
		}
		fp, err := safeJoin(".", name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fp, os.ModePerm); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fp), os.ModePerm); err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(fp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			_ = src.Close()
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			_ = dst.Close()
			_ = src.Close()
			return err
		}
		_ = dst.Close()
		_ = src.Close()
	}
	return nil
}

func zipEntryName(f *zip.File) string {
	name := f.Name
	if f.NonUTF8 {
		if fixed, ok := tryFixZipName(name); ok {
			name = fixed
		}
	}
	return name
}

func safeJoin(baseDir, name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if name == "" {
		return "", fmt.Errorf("路径为空")
	}
	if filepath.IsAbs(name) || filepath.VolumeName(name) != "" || strings.HasPrefix(name, "//") {
		return "", fmt.Errorf("非法路径: %s", name)
	}
	cleanName := filepath.Clean(filepath.FromSlash(name))
	if cleanName == "." || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) || cleanName == ".." {
		return "", fmt.Errorf("路径逃逸: %s", name)
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(baseAbs, cleanName)
	joinedAbs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, joinedAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("路径逃逸: %s", name)
	}
	return joinedAbs, nil
}

func findZipEntry(zipPath, target string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	var matches []string
	for _, f := range r.File {
		name := zipEntryName(f)
		if filepath.Base(name) == target {
			if _, err := safeJoin(".", name); err != nil {
				return "", err
			}
			matches = append(matches, name)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("压缩包内未找到 %s", target)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("压缩包内存在多个 %s: %s", target, strings.Join(matches, ", "))
	}
	return matches[0], nil
}

func tryFixZipName(name string) (string, bool) {
	rawStr, err := charmap.CodePage437.NewEncoder().String(name)
	if err != nil {
		return "", false
	}
	raw := []byte(rawStr)

	if out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw); err == nil {
		return string(out), true
	}
	if out, err := simplifiedchinese.GBK.NewDecoder().Bytes(raw); err == nil {
		return string(out), true
	}
	return "", false
}

func findFileRecursive(root, target string) (string, bool) {
	var found string
	var errFound = errors.New("found")
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(p) == target {
			found = p
			return errFound
		}
		return nil
	})
	if found != "" {
		return found, true
	}
	return "", false
}
