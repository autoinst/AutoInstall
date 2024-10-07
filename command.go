package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func Command() {
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
		log.Printf("无法写入:", err)
		return
	}
}

func Download() {
	fmt.Println("傻逼")
}
