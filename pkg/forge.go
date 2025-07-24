package pkg

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/autoinst/AutoInstall/core"
)

func ForgeB(config core.InstConfig, simpfun bool, mise bool) {
	if config.Version == "latest" {
		latestVersion, latestLoader, err := FetchLatestForgeVersion()
		if err != nil {
			log.Println("获取最新 Forge 版本失败:", err)
			return
		}
		config.Version = latestVersion
		config.LoaderVersion = latestLoader
	}

	if config.LoaderVersion == "latest" {
		latestLoader, err := FetchLatestForgeLoaderForVersion(config.Version)
		if err != nil {
			log.Println("获取指定 Minecraft 版本的最新 Forge加载器 失败:", err)
			return
		}
		config.LoaderVersion = latestLoader
	}

	var installerURL string
	if config.Download == "bmclapi" {
		installerURL = fmt.Sprintf(
			"https://bmclapi2.bangbang93.com/maven/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
			config.Version, config.LoaderVersion, config.Version, config.LoaderVersion,
		)
	} else {
		installerURL = fmt.Sprintf(
			"https://maven.minecraftforge.net/maven/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
			config.Version, config.LoaderVersion, config.Version, config.LoaderVersion,
		)
	}
	installerPath := filepath.Join("./.autoinst/cache", fmt.Sprintf("forge-%s-%s-installer.jar", config.Version, config.LoaderVersion))
	fmt.Println("当前为 forge 加载器，正在下载:", installerURL)
	if err := core.DownloadFile(installerURL, installerPath); err != nil {
		log.Println("下载 forge 失败:", err)
		return
	}
	fmt.Println("forge 安装器下载完成:", installerPath)

	// 提取 version.json
	versionInfo, err := core.ExtractVersionJson(installerPath)
	if err != nil {
		log.Println("提取 version.json 失败:", err)
		return
	}

	librariesDir := "./libraries"
	if err := DownloadLibraries(versionInfo, librariesDir, config.MaxConnections, config.Download); err != nil {
		log.Println("下载库文件失败:", err)
		return
	}

	if config.Download == "bmclapi" {
		if err := DownloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
			log.Println("下载 mc 服务端失败:", err)
			return
		}
	}

	fmt.Println("库文件下载完成")
	if err := core.RunInstaller(installerPath, config.Loader, config.Version, config.LoaderVersion, config.Download, simpfun, mise); err != nil {
		log.Println("运行安装器失败:", err)
	}
	core.RunScript(config.Version, config.Loader, config.LoaderVersion, simpfun, mise, config.Argsment)
}

func FetchLatestForgeVersion() (string, string, error) {
	resp, err := http.Get("https://bmclapi2.bangbang93.com/forge/latest")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var result struct {
		Build struct {
			McVersion string `json:"mcversion"`
			Version   string `json:"version"`
		} `json:"build"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	return result.Build.McVersion, result.Build.Version, nil
}

func FetchLatestForgeLoaderForVersion(mcVersion string) (string, error) {
	url := fmt.Sprintf("https://bmclapi2.bangbang93.com/forge/minecraft/%s", mcVersion)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var builds []struct {
		Version string `json:"version"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&builds); err != nil {
		return "", err
	}

	if len(builds) == 0 {
		return "", fmt.Errorf("没有找到 Minecraft 版本 %s 的 Forge 版本", mcVersion)
	}

	sort.Slice(builds, func(i, j int) bool {
		return CompareForgeVersions(builds[i].Version, builds[j].Version) > 0
	})

	return builds[0].Version, nil
}

func CompareForgeVersions(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		aNum, _ := strconv.Atoi(aParts[i])
		bNum, _ := strconv.Atoi(bParts[i])

		if aNum > bNum {
			return 1
		} else if aNum < bNum {
			return -1
		}
	}

	if len(aParts) > len(bParts) {
		return 1
	} else if len(aParts) < len(bParts) {
		return -1
	}

	return 0
}
