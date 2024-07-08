package main

import (
    "fmt";
    "os"
 )//导入

func main() {
    fmt.Println("[AutoInstall] 启动")
    fmt.Println("查找可执行文件中")
    if _, err := os.Stat("./fastinst.json"); err == nil {
      fmt.Printf("诶wc\n");
   } else {
      fmt.Printf("找不到\n");
   }
}
