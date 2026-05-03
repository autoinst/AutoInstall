package pkg

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/autoinst/AutoInstall/core"
)

func SPCInstall(file string, MaxCon int, Args string, bundleName string, MaxRetries int, baseConfig core.InstConfig) error {
	varsFile := "variables.txt"
	if file != "" && strings.HasSuffix(strings.ToLower(file), ".txt") {
		varsFile = file
	}
	if _, err := os.Stat(varsFile); os.IsNotExist(err) {
		return fmt.Errorf("当前目录下缺少 variables.txt 文件")
	}
	vars, err := readVariables(varsFile)
	if err != nil {
		return fmt.Errorf("读取 variables.txt 失败: %w", err)
	}
	version, okV := vars["MINECRAFT_VERSION"]
	loader, okL := vars["MODLOADER"]
	loaderVersion, okLV := vars["MODLOADER_VERSION"]

	if !okV || version == "" || !okL || loader == "" || !okLV || loaderVersion == "" {
		return fmt.Errorf("安装文件 variables.txt 中缺少必要配置项: MINECRAFT_VERSION, MODLOADER, MODLOADER_VERSION")
	}

	instConfig := modpackBaseConfig(baseConfig, MaxRetries)
	instConfig.Version = version
	instConfig.Loader = strings.ToLower(loader)
	instConfig.LoaderVersion = loaderVersion
	jsonData, err := json.MarshalIndent(instConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("生成 JSON 数据失败: %w", err)
	}
	if err := os.WriteFile("inst.json", jsonData, 0777); err != nil {
		return fmt.Errorf("写入 inst.json 文件失败: %w", err)
	}

	runInstalledModFilter(bundleName)
	return nil
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

		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

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
