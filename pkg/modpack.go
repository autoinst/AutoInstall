package pkg

import (
	"fmt"
	"os"
	"path/filepath"
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
		Modrinth("modpack.mrpack", MaxConnections, Argsment)
		return
	}
	if fileExists("modpack.zip", zipFiles) {
		fmt.Println("发现整合包: modpack.zip")
		SPCInstall("modpack.zip", MaxConnections, Argsment)
		return
	}

	if len(allPacks) == 1 {
		fmt.Println("发现整合包:", allPacks[0])
		handlePack(allPacks[0], MaxConnections, Argsment)
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
		SPCInstall(file, MaxConnections, Argsment)
	case ".mrpack":
		Modrinth(file, MaxConnections, Argsment)
	default:
		fmt.Println("未知文件类型:", file)
	}
}
