package pkg

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/autoinst/AutoInstall/core"
)

func SPCInstall(file string, MaxCon int, Args string) {
	if strings.HasSuffix(file, ".zip") {
		zipFile, err := zip.OpenReader(file)
		if err != nil {
			panic(err)
		}
		defer zipFile.Close()
		for _, f := range zipFile.File {
			filePath := filepath.Join("./", f.Name)
			// 忽略 start.sh 和 run.sh 脚本
			if f.Name == "start.sh" || f.Name == "run.sh" {
				fmt.Printf("跳过文件: %s\n", f.Name)
				continue
			}
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
	if _, err := os.Stat("variables.txt"); os.IsNotExist(err) {
		fmt.Println("错误：当前目录下缺少 variables.txt 文件。")
		return
	}
	vars, err := readVariables("variables.txt")
	if err != nil {
		fmt.Println("读取 variables.txt 失败:", err)
		return
	}
	instConfig := core.InstConfig{
		Version:        vars["MINECRAFT_VERSION"],
		Loader:         strings.ToLower(strings.ToLower(vars["MODLOADER"])),
		LoaderVersion:  vars["MODLOADER_VERSION"],
		Download:       "bmclapi",
		MaxConnections: 32,
		Argsment:       "-Xmx{maxmen}M -Xms{maxmen}M -XX:+AlwaysPreTouch -XX:+DisableExplicitGC -XX:+ParallelRefProcEnabled -XX:+PerfDisableSharedMem -XX:+UnlockExperimentalVMOptions -XX:+UseG1GC -XX:G1HeapRegionSize=8M -XX:G1HeapWastePercent=5 -XX:G1MaxNewSizePercent=40 -XX:G1MixedGCCountTarget=4 -XX:G1MixedGCLiveThresholdPercent=90 -XX:G1NewSizePercent=30 -XX:G1RSetUpdatingPauseTimePercent=5 -XX:G1ReservePercent=20 -XX:InitiatingHeapOccupancyPercent=15 -XX:MaxGCPauseMillis=200 -XX:MaxTenuringThreshold=1 -XX:SurvivorRatio=32 -Dusing.aikars.flags=https://mcflags.emc.gs -Daikars.new.flags=true",
	}
	jsonData, err := json.MarshalIndent(instConfig, "", "  ")
	if err != nil {
		fmt.Println("生成 JSON 数据失败:", err)
		return
	}
	err = os.WriteFile("inst.json", jsonData, 0777)
	if err != nil {
		fmt.Println("写入 inst.json 文件失败:", err)
		return
	}
}

func readVariables(filePath string) (map[string]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	variables := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// 忽略注释和空行
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// 解析键值对
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			variables[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return variables, nil
}
