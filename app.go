package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
) //导入
func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "正在扫描环境")
} // 测试是否为简幻欢服务器

// 重置已有日志
func renameFileToDateTime(oldFilePath string) (newFilePath string, err error) {
	// 获取当前日期
	date := time.Now().Format("2006-01-02")

	// 构建新的文件名
	newFileName := "Autoinst-" + date + ".txt"
	newFilePath = filepath.Join(filepath.Dir(oldFilePath), newFileName)

	// 检查文件是否存在，如果存在，则添加递增的数字
	counter := 1
	for {
		if _, err := os.Stat(newFilePath); os.IsNotExist(err) {
			break // 文件不存在，可以重命名
		}
		// 文件存在，添加数字后缀
		newFileName = fmt.Sprintf("Autoinst-%s_%d.txt", date, counter)
		newFilePath = filepath.Join(filepath.Dir(oldFilePath), newFileName)
		counter++
	}

	// 重命名文件
	err = os.Rename(oldFilePath, newFilePath)
	if err != nil {
		return "", err
	}

	return newFilePath, nil
}

func main() {
	logFilePath := "./.autoinst/logs/laster.txt"

	//整活下载器
	now := time.Now()
	currentDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	targetDate := time.Date(currentDate.Year(), time.April, 1, 0, 0, 0, 0, now.Location())
	if currentDate == targetDate {
		log.Println("正在初始化瑞典原神下崽器")
	} else {
		log.Println("AutoInstall初始化")
	}
	if _, err := os.Stat("./.autoinst"); err == nil {
		fmt.Println("OK")
	} else {
		os.MkdirAll(".autoinst", os.ModePerm)
		//检测是否为simpfun
		_, err := os.Stat("./.autoinst/simpfun")
		if err == nil {
			log.Printf("OK")
		} else {
			port := os.Getenv("SERVER_PORT")
			if port == "" {
				// 设置HTTP服务器
				http.HandleFunc("/", handler)
				server := &http.Server{Addr: ":" + port}
				// 启动服务器
				go func() {
					log.Println("服务器启动测试")
					if err := server.ListenAndServe(); err != http.ErrServerClosed {
						log.Fatalf("监听出现一些错误: %v", err)
					}
				}()
				// 给服务器一点时间来启动
				time.Sleep(1 * time.Second)
				// 构建请求
				url := "http://play.simpfun.cn:" + port
				_, err = http.Get(url)
				if err != nil {
					log.Printf("验证失败: %v", err)
				} else {
					log.Println("你正在简幻欢的服务器上使用AutoInstall,将使用预设")
					//记录
					file, err := os.Create("./.autoinst/simpfun")
					if err != nil {
						log.Fatalf("无法创建文件: %v", err)
					}
					defer file.Close()
				}
				if err := server.Shutdown(nil); err != nil {
					log.Fatalf("强制关闭测试服务器: %v", err)
				}
				log.Println("测试服务器已关闭")
			}
		}
	}

	if _, err := os.Stat("./.autoinst/logs"); err == nil {
		if _, err := os.Stat("./.autoinst/logs/laster.txt"); err == nil {
			oldFilePath := "./.autoinst/logs/laster.txt"
			// 调用函数重命名文件
			newFilePath, err := renameFileToDateTime(oldFilePath)
			if err != nil {
				fmt.Println("无法重置日志:", err)
				return
			}
			fmt.Println("日志重置完成:", newFilePath)
		}
		os.MkdirAll(".autoinst/logs", os.ModePerm)
	}
	logFile, err := os.Create(logFilePath)
	if err != nil {
		log.Fatalf("无法创建日志文件: %v", err)
		os.MkdirAll(".autoinst/logs", os.ModePerm)
	}

	// 设置日志输出到文件
	log.SetOutput(logFile)

	if _, err := os.Stat("./.autoinst/cache"); err == nil {
		log.Println("已有缓存文件")
	} else {
		os.MkdirAll(".autoinst/cache", os.ModePerm)
	}
	//开始安装
	log.Printf("启动方式\n")
	log.Printf("1.WEB操作(1)\n")
	log.Printf("2.命令行启动(2)\n")
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("读取输入时发生错误:", err)
		return
	}
	text = strings.TrimSpace(text)
	if text == "1" {
		log.Printf("帮我写Vue?\n")
		log.Printf("了解一下https://github.com/jdnjk/autoinst_web\n")
		log.Printf("10秒后跳转到命令行\n")
		time.Sleep(10 * time.Second)
	} else if text == "2" {
		log.Printf("启动命令行\n")
	} else {
		log.Printf("?你在干啥\n")
		os.Exit(0)
	}
	dir := "./.autoinst/cache"
	filename := "version_manifest_v2.json"
	filePath := filepath.Join(dir, filename)

	// 获取文件
	fmt.Println("获取mc版本")
	resp, err := http.Get("http://launchermeta.mojang.com/mc/game/version_manifest_v2.json")
	if err != nil {
		log.Println("无法下载文件,Mojang服务器发力了:", err)
		return
	}
	defer resp.Body.Close()
	// 检查服务器响应状态码
	if resp.StatusCode != http.StatusOK {
		log.Println("无法下载文件，当前状态码为:", resp.StatusCode)
		return
	}
	// 创建要保存的文件
	file, err := os.Create(filePath)
	if err != nil {
		log.Println("权限不足或因屎山代码而无法创建文件:", err)
		return
	}
	defer file.Close()
	// 将下载的内容写入文件
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		log.Println("无法写入:", err)
		return
	}
}
