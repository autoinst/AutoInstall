package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
) //导入

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

	fmt.Println("AutoInstall初始化")
	if _, err := os.Stat("./.autoinst"); err == nil {
		fmt.Println("OK")
	} else {
		os.MkdirAll(".autoinst", os.ModePerm)
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
	} else {
		os.MkdirAll(".autoinst/logs", os.ModePerm)
	}
	logFile, err := os.Create(logFilePath)

	// 设置日志输出到文件
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(multiWriter)

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
		//cmd.InitBase()
	} else {
		log.Printf("?你在干啥\n")
		os.Exit(0)
	}
}
