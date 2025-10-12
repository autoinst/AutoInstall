package pkg

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func Search(MaxConnections int, Argsment string) {
	fmt.Println("正在扫描可用的整合包...")
	if _, err := os.Stat("variables.txt"); err == nil {
		fmt.Println("检测到 variables.txt")
		SPCInstall("variables.txt", MaxConnections, Argsment)
		return
	}

	if _, err := os.Stat("modrinth.index.json"); err == nil {
		fmt.Println("检测到 modrinth.index.json")
		Modrinth("modrinth.index.json", MaxConnections, Argsment)
		return
	}

	mrpackFiles, _ := filepath.Glob("*.mrpack")
	zipFiles, _ := filepath.Glob("*.zip")

	allPacks := append([]string{}, mrpackFiles...)
	allPacks = append(allPacks, zipFiles...)

	allFiles := append(append([]string{}, mrpackFiles...), zipFiles...)

	if len(allFiles) == 0 && len(allPacks) == 0 {
		fmt.Println("未找到整合包")
		return
	}

	if fileExists("modpack.mrpack", mrpackFiles) {
		fmt.Println("发现整合包: modpack.mrpack")
		if err := extractZip("modpack.mrpack", nil); err != nil {
			fmt.Println("解压失败:", err)
			return
		}
		Modrinth("modrinth.index.json", MaxConnections, Argsment)
		return
	}
	if fileExists("modpack.zip", zipFiles) {
		fmt.Println("发现整合包: modpack.zip")
		// 若为 CurseForge 包（含 manifest.json），走 CurseForge，否则走 SPCInstall
		if zipContains("modpack.zip", "manifest.json") {
			if err := extractZip("modpack.zip", nil); err != nil {
				fmt.Println("解压失败:", err)
				return
			}
			if p, ok := findFileRecursive(".", "manifest.json"); ok {
				CurseForge(p, MaxConnections, Argsment)
			} else {
				CurseForge("manifest.json", MaxConnections, Argsment)
			}
		} else {
			if err := extractZip("modpack.zip", map[string]struct{}{"start.sh": {}, "run.sh": {}}); err != nil {
				fmt.Println("解压失败:", err)
				return
			}
			if p, ok := findFileRecursive(".", "variables.txt"); ok {
				SPCInstall(p, MaxConnections, Argsment)
			} else {
				SPCInstall("variables.txt", MaxConnections, Argsment)
			}
		}
		return
	}

	if len(allPacks) == 1 {
		fmt.Println("发现整合包:", allPacks[0])
		// 单一 zip 时尝试识别 CF
		if filepath.Ext(allPacks[0]) == ".zip" && zipContains(allPacks[0], "manifest.json") {
			if err := extractZip(allPacks[0], nil); err != nil {
				fmt.Println("解压失败:", err)
				return
			}
			if p, ok := findFileRecursive(".", "manifest.json"); ok {
				CurseForge(p, MaxConnections, Argsment)
			} else {
				CurseForge("manifest.json", MaxConnections, Argsment)
			}
		} else {
			handlePack(allPacks[0], MaxConnections, Argsment)
		}
		return
	}

	if len(allPacks) > 1 {
		fmt.Println("发现多个整合包，但未找到 modpack.zip 或 modpack.mrpack")
		fmt.Println("请将要使用的整合包重命名为 modpack.zip 或 modpack.mrpack 后重试")
		for _, file := range allPacks {
			fmt.Println("  " + file)
		}
		os.Exit(1)
	}
}

func fileExists(target string, list []string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

func handlePack(file string, MaxConnections int, Argsment string) {
	switch filepath.Ext(file) {
	case ".zip":
		if zipContains(file, "manifest.json") {
			if err := extractZip(file, nil); err != nil {
				fmt.Println("解压失败:", err)
				return
			}
			if p, ok := findFileRecursive(".", "manifest.json"); ok {
				CurseForge(p, MaxConnections, Argsment)
			} else {
				CurseForge("manifest.json", MaxConnections, Argsment)
			}
		} else {
			if err := extractZip(file, map[string]struct{}{"start.sh": {}, "run.sh": {}}); err != nil {
				fmt.Println("解压失败:", err)
				return
			}
			if p, ok := findFileRecursive(".", "variables.txt"); ok {
				SPCInstall(p, MaxConnections, Argsment)
			} else {
				SPCInstall("variables.txt", MaxConnections, Argsment)
			}
		}
	case ".mrpack":
		if err := extractZip(file, nil); err != nil {
			fmt.Println("解压失败:", err)
			return
		}
		if p, ok := findFileRecursive(".", "modrinth.index.json"); ok {
			Modrinth(p, MaxConnections, Argsment)
		} else {
			Modrinth("modrinth.index.json", MaxConnections, Argsment)
		}
	default:
		fmt.Println("未知文件类型:", file)
	}
}

// zipContains 检查 zip 包内是否包含指定路径（简单前缀匹配不做目录规范化）
func zipContains(zipPath, target string) bool {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return false
	}
	defer r.Close()
	for _, f := range r.File {
		// 直接匹配
		if f.Name == target {
			return true
		}
		// 若名称编码标记为非 UTF-8，则尝试 CP437 -> GB18030 修正后再比对
		if f.NonUTF8 {
			if fixed, ok := tryFixZipName(f.Name); ok && fixed == target {
				return true
			}
		}
		// 匹配 basename（适配顶层文件夹包内路径如 xxx/manifest.json）
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

// extractZip 将 zip/mrpack 解压到当前目录，支持按文件名跳过指定文件。
// 如果 skipNames 传入为 nil，则不跳过任何文件；用 map[string]struct{}{"a":{},"b":{}} 表示跳过 a 和 b。
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
				// 跳过
				continue
			}
			if _, ok := skipNames[filepath.Base(name)]; ok {
				// 跳过
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

// tryFixZipName 尝试将被按 CP437 解出的名称，还原为原始字节后按 GB18030/GBK 解码
// 适用于 Windows 上使用本地编码(GBK/GB18030)打包而未设置 UTF-8 标志的压缩包
func tryFixZipName(name string) (string, bool) {
	// 将当前 UTF-8 字符串按 CP437 重新编码回原始字节序列
	// 注：charmap.CodePage437 的 Encoder 会把可映射的 Unicode 转回 CP437 字节
	rawStr, err := charmap.CodePage437.NewEncoder().String(name)
	if err != nil {
		return "", false
	}
	raw := []byte(rawStr)

	// 优先 GB18030（向下兼容 GBK）
	if out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw); err == nil {
		return string(out), true
	}
	// 退回 GBK
	if out, err := simplifiedchinese.GBK.NewDecoder().Bytes(raw); err == nil {
		return string(out), true
	}
	return "", false
}

// findFileRecursive 在 root 下递归查找名为 target 的文件（按 basename 比较），找到即返回其相对路径
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
