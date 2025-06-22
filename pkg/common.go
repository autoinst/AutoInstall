package pkg

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"

	"github.com/autoinst/AutoInstall/core"
)

func Common(config core.InstConfig, cleaninst bool) {
	javaPath, simpfun, mise := core.FindJava()
	if simpfun {
		fmt.Println("已启用 simpfun 环境")
		if mise {
			fmt.Println("启用mise")
		}
	} else {
		if javaPath == "" {
			log.Println("未找到 Java，请确保已安装 Java 并设置 PATH。")
			return
		}
		fmt.Println("找到 Java 运行环境:", javaPath)
	}

	if config.Version != "latest" {
		matched, _ := regexp.MatchString(`[a-zA-Z]`, config.Version)
		if matched {
			if config.Loader == "neoforge" || config.Loader == "forge" {
				fmt.Println("安装器不支持安装(Neo)Forge快照/愚人节版本，请使用原版或 Fabric 加载器。")
				os.Exit(128)
			}
		}
	}
	if config.Loader == "neoforge" {
		NeoForgeB(config, simpfun, mise)
	} else if config.Loader == "forge" {
		ForgeB(config, simpfun, mise)
	}

	if config.Loader == "fabric" {
		FabricB(config, simpfun, mise)
	}
	if config.Loader == "vanilla" {
		if config.Version == "latest" {
			latestSnapshot, err := FetchLatestVanillaVersion(config.Download)
			if err != nil {
				log.Println("获取最新 Minecraft 版本失败:", err)
				return
			}
			config.Version = latestSnapshot
		}

		librariesDir := "./libraries"
		if err := DownloadServerJar(config.Version, config.Loader, librariesDir); err != nil {
			log.Println("下载 mc 服务端失败:", err)
			return
		}
		fmt.Println("服务端下载完成")
		core.RunScript(config.Version, config.Loader, config.LoaderVersion, simpfun, mise)
	}

	if cleaninst {
		fmt.Println("正在清理残留...")
		_ = os.Remove("modrinth.index.json")
		if err := os.Remove(".autoinst"); err != nil && !os.IsNotExist(err) {
			log.Println("删除 .autoinst 文件失败:", err)
		}
		files, err := os.ReadDir(".")
		if err == nil {
			for _, file := range files {
				if !file.IsDir() && len(file.Name()) > 4 && file.Name()[len(file.Name())-4:] == ".log" {
					if err := os.Remove(file.Name()); err != nil {
						fmt.Println("删除日志文件失败:", file.Name(), err)
					}
				}
			}
		} else {
			log.Println("读取目录失败:", err)
		}
		fmt.Println("清理完成")
	}
}

func FetchLatestVanillaVersion(downloadSource string) (string, error) {
	var url string
	if downloadSource == "bmclapi" {
		url = "https://bmclapi2.bangbang93.com/mc/game/version_manifest.json"
	} else {
		url = "https://launchermeta.mojang.com/mc/game/version_manifest.json"
	}

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Latest struct {
			Snapshot string `json:"snapshot"`
		} `json:"latest"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Latest.Snapshot, nil
}
