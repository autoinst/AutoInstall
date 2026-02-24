package pkg

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/autoinst/AutoInstall/core"
)

const (
	defaultMavenBase = "https://repo1.maven.org/maven2/"
	launchJarName    = "fabric-server-launch.jar"
)

var sigFilePattern = regexp.MustCompile(`(?i)^META-INF/[^/]+\.(SF|DSA|RSA|EC)$`)

type rawLibrary map[string]interface{}

type metaResult struct {
	Libraries []rawLibrary
	MainClass interface{} // 可能是字符串或包含 server 键的对象
}

type parsedLib struct {
	Name      string // Maven 坐标（group:artifact:version）
	RelPath   string // Maven 相对路径（如 net/fabricmc/fabric-loader/0.14.21/fabric-loader-0.14.21.jar）
	URL       string // 完整下载 URL
	InputPath string // 本地输入路径（如果提供）
	Skip      bool   // 跳过标记（如 native）
}

func FabricB(config core.InstConfig, simpfun bool, mise bool) {
	if config.Version == "latest" {
		latestRelease, err := FetchLatestFabricMinecraftVersion()
		if err != nil {
			core.Log("获取最新我的世界版本失败:", err)
			return
		}
		config.Version = latestRelease
		stableLoader, err := FetchLatestStableFabricLoaderVersion()
		if err != nil {
			core.Log("获取最新 Fabric Loader 版本失败:", err)
			return
		}
		config.LoaderVersion = stableLoader
	}
	if config.LoaderVersion == "latest" {
		stableLoader, err := FetchLatestStableFabricLoaderVersion()
		if err != nil {
			core.Log("获取最新 Fabric Loader 版本失败:", err)
			return
		}
		config.LoaderVersion = stableLoader
	}

	baseDir := "."
	libsDir := filepath.Join(baseDir, "libraries")

	client := &http.Client{Timeout: 30 * time.Second}
	meta, err := queryLoaderServerMeta(client, config.Version, config.LoaderVersion)
	if err != nil {
		core.Log("查询元数据失败:", err)
		return
	}

	libs, err := parseLibraries(meta)
	if err != nil {
		core.Log("解析库失败:", err)
		return
	}

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		core.Log("创建基础目录失败:", err)
		return
	}
	if err := os.MkdirAll(libsDir, 0o755); err != nil {
		core.Log("创建库目录失败:", err)
		return
	}

	if err := DownloadServerJar(config.Version, config.Loader, libsDir); err != nil {
		core.Log("下载服务端失败:", err)
		return
	}

	var libraryFiles []string
	var wg sync.WaitGroup

	maxThreads := config.MaxConnections
	if maxThreads <= 0 {
		maxThreads = 4
	}

	semaphore := make(chan struct{}, maxThreads)
	errChan := make(chan error, len(libs))

	for _, lib := range libs {
		if lib.Skip {
			continue
		}

		target := filepath.Join(libsDir, filepath.FromSlash(lib.RelPath))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			core.Log("创建库目录失败:", err)
			return
		}
		libraryFiles = append(libraryFiles, target)

		wg.Add(1)
		go func(filePath, libInputPath, libURL string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if libInputPath != "" {
				core.Log("复制本地库:", libInputPath, "->", filePath)
				if err := copyFileFabric(libInputPath, filePath); err != nil {
					errChan <- fmt.Errorf("复制库 %s 失败: %w", libInputPath, err)
				}
			} else {
				core.Log("下载库:", libURL, "->", filePath)
				if err := downloadFileFabric(client, libURL, filePath); err != nil {
					errChan <- fmt.Errorf("下载库 %s 失败: %w", libURL, err)
				}
			}
		}(target, lib.InputPath, lib.URL)
	}

	wg.Wait()
	close(errChan)
	for err := range errChan {
		if err != nil {
			core.Log("库下载失败:", err)
			return
		}
	}

	// 确定启动类
	launchMainClass := extractLaunchMainClass(meta)
	if launchMainClass == "" {
		core.Log("警告: 无法从元数据确定启动 mainClass，将留空")
	}
	jarMainClass := "net.fabricmc.loader.launch.server.FabricServerLauncher"

	launchJarPath := filepath.Join(baseDir, launchJarName)
	core.Log("生成启动 jar:", launchJarPath)
	if err := makeLaunchJar(launchJarPath, launchMainClass, jarMainClass, libraryFiles, false); err != nil {
		core.Log("生成启动 jar 失败:", err)
		return
	}

	core.Log("Fabric 安装完成!")
	core.RunScript(config.Version, config.Loader, config.LoaderVersion, simpfun, mise, config.Argsment)
}

func FetchLatestFabricMinecraftVersion() (string, error) {
	resp, err := http.Get("https://launchermeta.mojang.com/mc/game/version_manifest.json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Latest struct {
			Release string `json:"release"`
		} `json:"latest"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Latest.Release, nil
}

func FetchLatestStableFabricLoaderVersion() (string, error) {
	resp, err := http.Get("https://meta.fabricmc.net/v2/versions/loader")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var versions []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return "", err
	}

	for _, v := range versions {
		if v.Stable {
			return v.Version, nil
		}
	}

	return "", fmt.Errorf("未找到稳定版本的Fabric Loader")
}

func queryLoaderServerMeta(client *http.Client, mcVersion, loaderVersion string) (*metaResult, error) {
	url := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/server/json", mcVersion, loaderVersion)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("meta responded %d: %s", resp.StatusCode, string(b))
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	meta := &metaResult{}

	if libs, ok := raw["libraries"]; ok {
		if arr, ok := libs.([]interface{}); ok {
			for _, e := range arr {
				if m, ok := e.(map[string]interface{}); ok {
					meta.Libraries = append(meta.Libraries, rawLibrary(m))
				}
			}
		}
	}

	if mc, ok := raw["mainClass"]; ok {
		meta.MainClass = mc
	}

	return meta, nil
}

func parseLibraries(meta *metaResult) ([]parsedLib, error) {
	var out []parsedLib
	for _, rl := range meta.Libraries {
		if _, has := rl["natives"]; has {
			out = append(out, parsedLib{Skip: true, Name: libNameFromRaw(rl)})
			continue
		}

		name := libNameFromRaw(rl)
		if name == "" {
			if p, ok := rl["path"].(string); ok && p != "" {
				out = append(out, parsedLib{
					Name:    p,
					RelPath: p,
					URL:     "",
				})
				continue
			}
			continue
		}

		rel := mavenRelPathFromName(name)
		url := ""
		inputPath := ""

		if downloads, ok := rl["downloads"].(map[string]interface{}); ok {
			if artifact, ok := downloads["artifact"].(map[string]interface{}); ok {
				if u, ok := artifact["url"].(string); ok && u != "" {
					url = u
				}
				if p, ok := artifact["path"].(string); ok && p != "" && url == "" {
					url = defaultMavenBase + p
				}
			}
		}

		if u, ok := rl["url"].(string); ok && u != "" {
			base := u
			if !strings.HasSuffix(base, "/") {
				base += "/"
			}
			url = base + rel
		}

		if p, ok := rl["path"].(string); ok && p != "" {
			rel = p
		}

		if url == "" {
			url = defaultMavenBase + rel
		}

		if ip, ok := rl["inputPath"].(string); ok && ip != "" {
			inputPath = ip
		}

		out = append(out, parsedLib{
			Name:      name,
			RelPath:   rel,
			URL:       url,
			InputPath: inputPath,
			Skip:      false,
		})
	}
	return out, nil
}

func libNameFromRaw(rl rawLibrary) string {
	if v, ok := rl["name"].(string); ok {
		return v
	}
	return ""
}

func mavenRelPathFromName(name string) string {
	parts := strings.Split(name, ":")
	if len(parts) < 3 {
		return name
	}
	group := parts[0]
	artifact := parts[1]
	version := parts[2]
	jarName := artifact + "-" + version + ".jar"
	groupPath := strings.ReplaceAll(group, ".", "/")
	return filepath.ToSlash(filepath.Join(groupPath, artifact, version, jarName))
}

func extractLaunchMainClass(meta *metaResult) string {
	if meta.MainClass == nil {
		return ""
	}
	switch v := meta.MainClass.(type) {
	case string:
		return v
	case map[string]interface{}:
		if s, ok := v["server"].(string); ok {
			return s
		}
		for _, val := range v {
			if s, ok := val.(string); ok {
				return s
			}
		}
	default:
		b, err := json.Marshal(v)
		if err == nil {
			var s string
			if json.Unmarshal(b, &s) == nil {
				return s
			}
		}
	}
	return ""
}

func downloadFileFabric(client *http.Client, url, target string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download %s returned %d: %s", url, resp.StatusCode, string(body))
	}

	tmp := target + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, resp.Body)
	if err2 := out.Close(); err == nil {
		err = err2
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, target)
}

func copyFileFabric(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func makeLaunchJar(file, launchMainClass, jarMainClass string, libraryFiles []string, shade bool) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}

	_ = os.Remove(file)
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	added := map[string]struct{}{}

	manifestPath := "META-INF/MANIFEST.MF"
	added[manifestPath] = struct{}{}
	w, err := zw.Create(manifestPath)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString("Manifest-Version: 1.0\r\n")
	if jarMainClass != "" {
		buf.WriteString("Main-Class: " + jarMainClass + "\r\n")
	}
	if !shade {
		relPaths := make([]string, 0, len(libraryFiles))
		for _, p := range libraryFiles {
			rel, err := filepath.Rel(filepath.Dir(file), p)
			if err != nil {
				rel = p
			}
			rel = filepath.ToSlash(rel)
			relPaths = append(relPaths, rel)
		}
		if len(relPaths) > 0 {
			buf.WriteString("Class-Path: " + strings.Join(relPaths, " ") + "\r\n")
		}
	}
	buf.WriteString("\r\n")
	if _, err := w.Write(buf.Bytes()); err != nil {
		return err
	}

	propsPath := "fabric-server-launch.properties"
	added[propsPath] = struct{}{}
	w2, err := zw.Create(propsPath)
	if err != nil {
		return err
	}
	props := "launch.mainClass=" + launchMainClass + "\n"
	if _, err := w2.Write([]byte(props)); err != nil {
		return err
	}

	if shade {
		services := map[string]map[string]struct{}{}
		bufCopy := make([]byte, 32*1024)
		for _, lib := range libraryFiles {
			zr, err := zip.OpenReader(lib)
			if err != nil {
				return fmt.Errorf("打开库 jar %s: %w", lib, err)
			}
			for _, ent := range zr.File {
				name := ent.Name
				if strings.HasSuffix(name, "/") {
					continue
				}
				if sigFilePattern.MatchString(name) {
					continue
				}
				if strings.HasPrefix(name, "META-INF/services/") && strings.Count(name[len("META-INF/services/"):], "/") == 0 {
					rc, err := ent.Open()
					if err != nil {
						_ = zr.Close()
						return err
					}
					content, _ := io.ReadAll(rc)
					_ = rc.Close()
					lines := strings.Split(string(content), "\n")
					set := services[name]
					if set == nil {
						set = map[string]struct{}{}
						services[name] = set
					}
					for _, line := range lines {
						line = strings.TrimSpace(line)
						if line == "" || strings.HasPrefix(line, "#") {
							continue
						}
						set[line] = struct{}{}
					}
					continue
				}
				if _, exists := added[name]; exists {
					continue
				}
				added[name] = struct{}{}
				rc, err := ent.Open()
				if err != nil {
					_ = zr.Close()
					return err
				}
				wr, err := zw.Create(name)
				if err != nil {
					_ = rc.Close()
					_ = zr.Close()
					return err
				}
				_, err = io.CopyBuffer(wr, rc, bufCopy)
				_ = rc.Close()
				if err != nil {
					_ = zr.Close()
					return err
				}
			}
			_ = zr.Close()
		}

		for name, set := range services {
			added[name] = struct{}{}
			wr, err := zw.Create(name)
			if err != nil {
				return err
			}
			for s := range set {
				if _, err := wr.Write([]byte(s + "\n")); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
