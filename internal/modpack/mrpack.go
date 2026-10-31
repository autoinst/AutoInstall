package modpack

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/autoinst/AutoInstall/internal/download"
	"github.com/autoinst/AutoInstall/internal/model"
	"github.com/autoinst/AutoInstall/internal/workspace"
)

type ModrinthFile struct {
	Path      string            `json:"path"`
	Env       map[string]string `json:"env"`
	Downloads []string          `json:"downloads"`
	Hashes    map[string]string `json:"hashes"`
}

type ModrinthIndex struct {
	FormatVersion int                  `json:"formatVersion"`
	Game          string               `json:"game"`
	VersionID     string               `json:"versionId"`
	Name          string               `json:"name"`
	Files         []ModrinthFile       `json:"files"`
	Dependencies  ModrinthDependencies `json:"dependencies"`
}

type ModrinthDependencies struct{ Minecraft, NeoForge, Forge, Fabric, FabricLoader string }

func (d *ModrinthDependencies) UnmarshalJSON(data []byte) error {
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for name := range raw {
		switch name {
		case "minecraft", "neoforge", "forge", "fabric", "fabric-loader":
		default:
			return fmt.Errorf("不支持的 Modrinth 依赖: %s", name)
		}
	}
	d.Minecraft = raw["minecraft"]
	d.NeoForge = raw["neoforge"]
	d.Forge = raw["forge"]
	d.Fabric = raw["fabric"]
	d.FabricLoader = raw["fabric-loader"]
	return nil
}

func modrinthTarget(index ModrinthIndex) (model.Target, error) {
	if index.FormatVersion != 0 && index.FormatVersion != 1 {
		return model.Target{}, fmt.Errorf("不支持的 Modrinth formatVersion: %d", index.FormatVersion)
	}
	if index.Game != "" && index.Game != "minecraft" {
		return model.Target{}, fmt.Errorf("不支持的游戏: %s", index.Game)
	}
	t := model.Target{Version: strings.TrimSpace(index.Dependencies.Minecraft), Loader: "vanilla"}
	for _, candidate := range []struct{ name, version string }{{"neoforge", index.Dependencies.NeoForge}, {"forge", index.Dependencies.Forge}, {"fabric", index.Dependencies.FabricLoader}, {"fabric", index.Dependencies.Fabric}} {
		if candidate.version != "" {
			t.Loader = candidate.name
			t.LoaderVersion = candidate.version
			break
		}
	}
	return t, nil
}

func planModrinth(w *workspace.Workspace, index ModrinthIndex, opts model.Options, deps Dependencies) ([]model.FileTask, error) {
	var files []model.FileTask
	for _, file := range index.Files {
		if strings.EqualFold(strings.TrimSpace(file.Env["server"]), "unsupported") {
			continue
		}
		if _, err := safeJoin(w.Root, file.Path); err != nil {
			return nil, err
		}
		if workspace.Protected(file.Path) {
			deps.log("跳过安装器控制文件:", file.Path)
			continue
		}
		if len(file.Downloads) == 0 {
			return nil, fmt.Errorf("文件 %s 没有下载链接", file.Path)
		}
		for _, raw := range file.Downloads {
			u, err := url.Parse(raw)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return nil, fmt.Errorf("文件 %s 下载链接不合法", file.Path)
			}
		}
		hashes := make(map[string]string)
		if sha := strings.TrimSpace(file.Hashes["sha512"]); sha != "" {
			hashes["sha512"] = sha
		} else if sha := strings.TrimSpace(file.Hashes["sha1"]); sha != "" {
			hashes["sha1"] = sha
		} else {
			return nil, fmt.Errorf("文件 %s 没有支持的校验哈希", file.Path)
		}
		for algorithm, value := range hashes {
			decoded, err := hex.DecodeString(value)
			length := 64
			if algorithm == "sha1" {
				length = 20
			}
			if err != nil || len(decoded) != length {
				return nil, fmt.Errorf("文件 %s 的 %s 哈希无效", file.Path, algorithm)
			}
		}
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(file.Path, "\\", "/"))))
		files = append(files, model.FileTask{Path: filepath.FromSlash(clean), URLs: file.Downloads, Hashes: hashes,
			NameFromURL: !opts.FollowModpackFilenames && strings.HasPrefix(clean, "mods/")})
	}
	return files, nil
}

func verifyModrinthFile(path string, hashes map[string]string) error {
	if sha := hashes["sha512"]; sha != "" {
		return download.VerifyFileHash(path, "sha512", sha)
	}
	if sha := hashes["sha1"]; sha != "" {
		return download.VerifyFileHash(path, "sha1", sha)
	}
	return fmt.Errorf("没有支持的校验哈希: %s", path)
}
