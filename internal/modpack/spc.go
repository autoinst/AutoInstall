package modpack

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/autoinst/AutoInstall/internal/model"
)

func spcTarget(vars map[string]string) (model.Target, error) {
	t := model.Target{Version: strings.TrimSpace(vars["MINECRAFT_VERSION"]), Loader: strings.ToLower(strings.TrimSpace(vars["MODLOADER"])), LoaderVersion: strings.TrimSpace(vars["MODLOADER_VERSION"])}
	if t.Version == "" || t.Loader == "" || (t.Loader != "vanilla" && t.LoaderVersion == "") {
		return t, fmt.Errorf("variables.txt 缺少 MINECRAFT_VERSION / MODLOADER / MODLOADER_VERSION")
	}
	if t.Loader == "vanilla" {
		t.LoaderVersion = ""
	}
	return t, nil
}

func readVariables(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseVariables(f)
}
func parseVariables(r io.Reader) (map[string]string, error) {
	vars := make(map[string]string)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		vars[strings.TrimSpace(key)] = value
	}
	return vars, scanner.Err()
}
