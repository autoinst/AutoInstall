package main

import (
    "fmt";
    "os";
    "bufio";
    "strings";
    "time";
    "io"
    "net/http";
	 "path/filepath"
 )//导入

func main() {
   now := time.Now()
	// 只保留年、月、日
	currentDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	// 设置比较的日期（4月1日）
	targetDate := time.Date(currentDate.Year(), time.April, 1, 0, 0, 0, 0, now.Location())
	// 比较当前日期和目标日期
	if currentDate == targetDate {
		fmt.Println("正在初始化瑞典原神下崽器")
	} else {
		fmt.Println("AutoInstall初始化ing")
	}
   fmt.Println("查找可执行文件中")
   if _, err := os.Stat("./.authinst"); err == nil {
      fmt.Printf("200 OK\n");
   } else {
      os.MkdirAll(".authinst", os.ModePerm)
   }
   fmt.Printf("启动方式\n");
   fmt.Printf("1.WEB操作(1)\n");
   fmt.Printf("2.命令行启动(2)\n");
   reader := bufio.NewReader(os.Stdin)
   text, err := reader.ReadString('\n')
   if err != nil {
      fmt.Println("读取输入时发生错误:", err)
      return
   }
   text = strings.TrimSpace(text)
   if text == "1" {
      fmt.Printf("帮我写Vue?\n");
      fmt.Printf("了解一下https://github.com/jdnjk/autoinst_web\n");
      fmt.Printf("10秒后跳转到命令行\n");
      time.Sleep(10 * time.Second)
	} else if text == "2" {
      fmt.Printf("启动命令行\n");
	} else {
      fmt.Printf("?你在干啥\n");
      os.Exit(0)
   }
   dir := "./.autoinst/cache"
	filename := "version_manifest_v2.json"
	filePath := filepath.Join(dir, filename)

   // 获取文件
   fmt.Println("获取mc版本")
   resp, err := http.Get("http://launchermeta.mojang.com/mc/game/version_manifest_v2.json")
	if err != nil {
		fmt.Println("无法下载文件,Mojang服务器发力了:", err)
		return
	}
	defer resp.Body.Close()
	// 检查服务器响应状态码
	if resp.StatusCode != http.StatusOK {
		fmt.Println("无法下载文件，当前状态码为:", resp.StatusCode)
		return
	}
	// 创建要保存的文件
	file, err := os.Create(filePath)
	if err != nil {
		fmt.Println("权限不足而无法创建文件:", err)
		return
	}
	defer file.Close()
	// 将下载的内容写入文件
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		fmt.Println("无法写入:", err)
		return
	}

   //处理json
   fmt.Println("处理文件:", err)
}
