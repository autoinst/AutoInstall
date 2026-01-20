package pkg

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"

	"github.com/autoinst/AutoInstall/core"
)

func SPCInstall(file string, MaxCon int, Args string) {
	varsFile := "variables.txt"
	if file != "" && strings.HasSuffix(strings.ToLower(file), ".txt") {
		varsFile = file
	}
	if _, err := os.Stat(varsFile); os.IsNotExist(err) {
		core.Log("错误：当前目录下缺少 variables.txt 文件。")
		return
	}
	vars, err := readVariables(varsFile)
	if err != nil {
		core.Log("读取 variables.txt 失败:", err)
		return
	}
	version, okV := vars["MINECRAFT_VERSION"]
	loader, okL := vars["MODLOADER"]
	loaderVersion, okLV := vars["MODLOADER_VERSION"]

	if !okV || version == "" || !okL || loader == "" || !okLV || loaderVersion == "" {
		core.Log("错误：安装文件 variables.txt 中缺少必要配置项")
		core.Log("请检查您的 variables.txt 是否包含以下内容：")
		core.Log("MINECRAFT_VERSION")
		core.Log("MODLOADER")
		core.Log("MODLOADER_VERSION")
		os.Exit(128)
	}

	instConfig := core.InstConfig{
		Version:        version,
		Loader:         strings.ToLower(loader),
		LoaderVersion:  loaderVersion,
		Download:       "bmclapi",
		MaxConnections: 32,
		Argsment:       "-Xmx{maxmen}M -Xms{maxmen}M -XX:+AlwaysPreTouch -XX:+DisableExplicitGC -XX:+ParallelRefProcEnabled -XX:+PerfDisableSharedMem -XX:+UnlockExperimentalVMOptions -XX:+UseG1GC -XX:G1HeapRegionSize=8M -XX:G1HeapWastePercent=5 -XX:G1MaxNewSizePercent=40 -XX:G1MixedGCCountTarget=4 -XX:G1MixedGCLiveThresholdPercent=90 -XX:G1NewSizePercent=30 -XX:G1RSetUpdatingPauseTimePercent=5 -XX:G1ReservePercent=20 -XX:InitiatingHeapOccupancyPercent=15 -XX:MaxGCPauseMillis=200 -XX:MaxTenuringThreshold=1 -XX:SurvivorRatio=32 -Dusing.aikars.flags=https://mcflags.emc.gs -Daikars.new.flags=true",
	}
	jsonData, err := json.MarshalIndent(instConfig, "", "  ")
	if err != nil {
		core.Log("生成 JSON 数据失败:", err)
		return
	}
	err = os.WriteFile("inst.json", jsonData, 0777)
	if err != nil {
		core.Log("写入 inst.json 文件失败:", err)
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
