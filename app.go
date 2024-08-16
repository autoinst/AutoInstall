package main

import (
    "fmt";
    "os";
    "net/http"
 )//导入

func main() {
    aria2 := "https://www.twle.cn/static/i/img1.jpg"
    fmt.Println("[AutoInstall] 启动")
    fmt.Println("查找可执行文件中")
    if _, err := os.Stat("./.authinst"); err == nil {
      fmt.Printf("ok\n");
   } else {
      fmt.Printf("创建执行文件\n");
      os.MkdirAll(".authinst/aria2", os.ModePerm)
}
