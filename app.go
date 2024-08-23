package main

import (
    "fmt";
    "os";
    "bufio";
    "strings"
 )//导入

func main() {
   fmt.Println("[AutoInstall] 启动")
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
   fmt.Printf("请问您想要什么版本\n");
   fmt.Printf("当前可用\n");
}
