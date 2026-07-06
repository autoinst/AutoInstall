package pkg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/autoinst/AutoInstall/core"
)

func DownloadServerJar(version, loader, librariesDir string, downloadSource string, maxRetries int, serverJarPath string) error {
	serverPath, err := resolveServerJarPath(version, loader, librariesDir, serverJarPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(serverPath), os.ModePerm); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	officialURL, officialSHA1, err := getOfficialServerJarURL(version, maxRetries)
	if err != nil {
		return fmt.Errorf("获取官方源失败: %v", err)
	}

	mirrorURL := fmt.Sprintf("https://bmclapi2.bangbang93.com/version/%s/server", version)
	currentURL, fallbackURL := sourceURLPair(officialURL, mirrorURL, downloadSource)
	if err := downloadServerJarFromURL(currentURL, serverPath, officialSHA1, maxRetries); err == nil {
		core.Log("下载完成 Minecraft 服务端:", serverPath)
		return nil
	} else {
		core.Log("当前源下载 Minecraft 服务端失败:", err)
	}

	if fallbackURL != currentURL {
		core.Log("切换备用源下载 Minecraft 服务端:", fallbackURL)
		if err := downloadServerJarFromURL(fallbackURL, serverPath, officialSHA1, maxRetries); err == nil {
			core.Log("下载完成 Minecraft 服务端:", serverPath)
			return nil
		} else {
			return fmt.Errorf("备用源下载 Minecraft 服务端失败: %w", err)
		}
	}
	return fmt.Errorf("无法下载服务端文件 %s", serverPath)
}

func resolveServerJarPath(version, loader, librariesDir, serverJarPath string) (string, error) {
	if strings.TrimSpace(serverJarPath) != "" {
		path := strings.TrimSpace(serverJarPath)
		path = strings.ReplaceAll(path, "{LIBRARY_DIR}", librariesDir)
		path = strings.ReplaceAll(path, "{MINECRAFT_VERSION}", version)
		path = filepath.Clean(filepath.FromSlash(path))
		if filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("serverJarPath 指向安装目录外: %s", serverJarPath)
		}
		return path, nil
	}

	serverFileName := serverJarFileName(version, loader)
	if loader == "fabric" || loader == "vanilla" {
		return filepath.Join(".", serverFileName), nil
	}
	if loader == "forge" && compareMCVersion(version, "1.16.5") <= 0 {
		return filepath.Join(".", serverFileName), nil
	}
	return filepath.Join(librariesDir, "net", "minecraft", "server", version, serverFileName), nil
}

func serverJarFileName(version, loader string) string {
	switch loader {
	case "forge":
		if compareMCVersion(version, "1.20.4") >= 0 {
			return fmt.Sprintf("server-%s-bundled.jar", version)
		}
		if compareMCVersion(version, "1.16.5") <= 0 {
			return fmt.Sprintf("minecraft_server.%s.jar", version)
		}
		return fmt.Sprintf("server-%s.jar", version)
	case "fabric", "vanilla":
		return "server.jar"
	default:
		return fmt.Sprintf("server-%s.jar", version)
	}
}

func downloadServerJarFromURL(url, serverPath, sha1 string, maxRetries int) error {
	if err := core.DownloadFileRetry(url, serverPath, maxRetries); err != nil {
		return err
	}
	if err := core.VerifyFileHash(serverPath, "sha1", sha1); err != nil {
		_ = os.Remove(serverPath)
		return err
	}
	return nil
}

func compareMCVersion(a, b string) int {
	return compareVersionTokens(a, b)
}

func getOfficialServerJarURL(version string, maxRetries int) (string, string, error) {
	resp, err := core.GetWithRetry("https://launchermeta.mojang.com/mc/game/version_manifest.json", maxRetries)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var manifest struct {
		Versions []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"versions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return "", "", err
	}

	var versionURL string
	for _, v := range manifest.Versions {
		if v.ID == version {
			versionURL = v.URL
			break
		}
	}

	if versionURL == "" {
		return "", "", fmt.Errorf("未找到版本 %s", version)
	}

	resp2, err := core.GetWithRetry(versionURL, maxRetries)
	if err != nil {
		return "", "", err
	}
	defer resp2.Body.Close()

	var versionInfo struct {
		Downloads struct {
			Server struct {
				URL  string `json:"url"`
				SHA1 string `json:"sha1"`
			} `json:"server"`
		} `json:"downloads"`
	}

	if err := json.NewDecoder(resp2.Body).Decode(&versionInfo); err != nil {
		return "", "", err
	}

	if versionInfo.Downloads.Server.URL == "" {
		return "", "", fmt.Errorf("未找到版本 %s 的服务端下载 URL", version)
	}

	return versionInfo.Downloads.Server.URL, versionInfo.Downloads.Server.SHA1, nil
}

func getJSONWithFallback(currentURL, officialURL string, maxRetries int, target interface{}) error {
	resp, err := core.GetWithFallback(currentURL, officialURL, maxRetries)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(target)
}

func getJSONWithRetry(url string, maxRetries int, target interface{}) error {
	resp, err := core.GetWithRetry(url, maxRetries)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(target)
}
