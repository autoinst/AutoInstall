package modcheck

import (
	"archive/zip"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var tomlModIDPattern = regexp.MustCompile(`(?m)^\s*modId\s*=\s*"([^"]+)"`)

func ExtractMods(modsDir string) ([]ModFile, error) {
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	mods := make([]ModFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".jar") {
			continue
		}

		fullPath := filepath.Join(modsDir, entry.Name())
		mod, err := extractSingleMod(fullPath)
		if err != nil {
			continue
		}
		mods = append(mods, mod)
	}

	return mods, nil
}

func extractSingleMod(fullPath string) (ModFile, error) {
	hash, err := fileSHA1(fullPath)
	if err != nil {
		return ModFile{}, err
	}

	r, err := zip.OpenReader(fullPath)
	if err != nil {
		return ModFile{}, err
	}
	defer r.Close()

	mod := ModFile{
		Path: fullPath,
		Name: filepath.Base(fullPath),
		Hash: hash,
	}

	modIDSet := make(map[string]struct{})
	projectIDSet := make(map[string]struct{})

	for _, file := range r.File {
		if file.FileInfo().IsDir() {
			continue
		}

		nameLower := strings.ToLower(file.Name)
		baseLower := strings.ToLower(filepath.Base(file.Name))

		if strings.HasSuffix(baseLower, ".json") && strings.Contains(baseLower, "mixins") {
			if data, err := readZipText(file); err == nil {
				mod.Mixins = append(mod.Mixins, MixinFile{Name: file.Name, Data: data})
			}
		}

		switch baseLower {
		case "fabric.mod.json":
			if data, err := readZipText(file); err == nil {
				if id := parseFabricModID(data); id != "" {
					modIDSet[id] = struct{}{}
				}
			}
		case "quilt.mod.json":
			if data, err := readZipText(file); err == nil {
				if id := parseQuiltModID(data); id != "" {
					modIDSet[id] = struct{}{}
				}
			}
		case "mcmod.info":
			if data, err := readZipText(file); err == nil {
				for _, id := range parseMcmodInfoIDs(data) {
					modIDSet[id] = struct{}{}
				}
			}
		case "mods.toml", "neoforge.mods.toml":
			if data, err := readZipText(file); err == nil {
				for _, id := range parseTomlModIDs(data) {
					modIDSet[id] = struct{}{}
				}
			}
		case "modrinth.index.json", "modrinth.json":
			if data, err := readZipText(file); err == nil {
				for _, id := range parseModrinthProjectIDs(data) {
					projectIDSet[id] = struct{}{}
				}
			}
		default:
			if strings.HasSuffix(nameLower, "/mods.toml") || strings.HasSuffix(nameLower, "/neoforge.mods.toml") {
				if data, err := readZipText(file); err == nil {
					for _, id := range parseTomlModIDs(data) {
						modIDSet[id] = struct{}{}
					}
				}
			}
		}
	}

	mod.ModIDs = setToSlice(modIDSet)
	mod.ProjectIDs = setToSlice(projectIDSet)
	return mod, nil
}

func readZipText(file *zip.File) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	b, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func fileSHA1(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func parseFabricModID(data string) string {
	var meta struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(data), &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.ID)
}

func parseQuiltModID(data string) string {
	var meta struct {
		QuiltLoader struct {
			ID string `json:"id"`
		} `json:"quilt_loader"`
	}
	if err := json.Unmarshal([]byte(data), &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.QuiltLoader.ID)
}

func parseMcmodInfoIDs(data string) []string {
	type mcmodEntry struct {
		ModID string `json:"modid"`
	}

	var list []mcmodEntry
	if err := json.Unmarshal([]byte(data), &list); err == nil {
		ids := make([]string, 0, len(list))
		for _, entry := range list {
			if id := strings.TrimSpace(entry.ModID); id != "" {
				ids = append(ids, id)
			}
		}
		return ids
	}

	var single mcmodEntry
	if err := json.Unmarshal([]byte(data), &single); err == nil {
		if id := strings.TrimSpace(single.ModID); id != "" {
			return []string{id}
		}
	}

	return nil
}

func parseTomlModIDs(data string) []string {
	matches := tomlModIDPattern.FindAllStringSubmatch(data, -1)
	if len(matches) == 0 {
		return nil
	}

	ids := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		id := strings.TrimSpace(match[1])
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func parseModrinthProjectIDs(data string) []string {
	ids := make([]string, 0, 2)
	seen := map[string]struct{}{}

	var meta map[string]any
	if err := json.Unmarshal([]byte(data), &meta); err != nil {
		return nil
	}

	appendID := func(value any) {
		id, ok := value.(string)
		id = strings.TrimSpace(id)
		if !ok || id == "" {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	appendID(meta["project_id"])
	appendID(meta["id"])

	if project, ok := meta["project"].(map[string]any); ok {
		appendID(project["id"])
	}

	return ids
}

func setToSlice(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	return out
}
