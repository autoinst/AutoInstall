package main

import (
    "fmt";
    "os";
    "net/http"
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
   scanner.Scan()
   start := scanner.Text()
   if start = 0 {

	} else if start = 1{

	} else {
      fmt.Printf("?你在干啥\n");
      os.Exit(0)
   }
}
