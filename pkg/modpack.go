package pkg

import (
	"archive/zip"
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

func WaitDownloads() {
	DownloadWg.Wait()
}

func Search(MaxConnections int, Argsment string) {
	core.Log("正在扫描可用的整合包...")
	pack, packType, err := detectPackFile()
	if err != nil {
		core.Log(err.Error())
		return
	}

	if err := installByType(packType, pack, MaxConnections, Argsment); err != nil {
		core.Log("安装失败:", err)
	}
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

func installByType(packType, path string, MaxConnections int, Argsment string) error {
	switch packType {
	case "spc-plain":
		SPCInstall(path, MaxConnections, Argsment)
		return nil
	case "modrinth-plain":
		Modrinth(path, MaxConnections, Argsment)
		return nil
	case "curseforge-plain":
		CurseForge(path, MaxConnections, Argsment)
		return nil
	case "modrinth":
		return installModrinthArchive(path, MaxConnections, Argsment)
	case "curseforge-zip":
		return installCurseForgeArchive(path, MaxConnections, Argsment)
	case "zip":
		return installZipArchive(path, MaxConnections, Argsment)
	default:
		return fmt.Errorf("无法识别的整合包类型: %s", packType)
	}
}

// installZipArchive 尝试识别 zip 是 CurseForge 还是 SPC 格式
func installZipArchive(file string, MaxConnections int, Argsment string) error {
	if zipContains(file, "manifest.json") {
		return installCurseForgeArchive(file, MaxConnections, Argsment)
	}
	return installSPCArchive(file, MaxConnections, Argsment)
}

func installCurseForgeArchive(file string, MaxConnections int, Argsment string) error {
	if err := extractZip(file, nil); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}
	manifestPath, ok := findFileRecursive(".", "manifest.json")
	if !ok {
		return errors.New("解压后未找到 manifest.json")
	}
	CurseForge(manifestPath, MaxConnections, Argsment)
	return nil
}

func installSPCArchive(file string, MaxConnections int, Argsment string) error {
	skipScripts := map[string]struct{}{"start.sh": {}, "run.sh": {}}
	if err := extractZip(file, skipScripts); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}
	variablesPath, ok := findFileRecursive(".", "variables.txt")
	if !ok {
		return errors.New("解压后未找到 variables.txt")
	}
	SPCInstall(variablesPath, MaxConnections, Argsment)
	return nil
}

func installModrinthArchive(file string, MaxConnections int, Argsment string) error {
	if err := extractZip(file, nil); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}
	indexPath, ok := findFileRecursive(".", "modrinth.index.json")
	if !ok {
		return errors.New("解压后未找到 modrinth.index.json")
	}
	Modrinth(indexPath, MaxConnections, Argsment)
	return nil
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
		name := f.Name
		if f.NonUTF8 {
			if fixed, ok := tryFixZipName(name); ok {
				name = fixed
			}
		}
		if skipNames != nil {
			if _, ok := skipNames[name]; ok {
				continue
			}
			if _, ok := skipNames[filepath.Base(name)]; ok {
				continue
			}
		}
		fp := filepath.Join("./", name)
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
