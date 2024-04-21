package main

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
) //导入

func main() {
    fmt.Println("[AutoInstall] 启动")
    fmt.Println("查找pack.zip")
    fileInfo, err := os.Stat("pack.zip")
    if err != nil {
        if os.IsNotExist(err) {
            log.Fatal("无法找到文件")
        }
    }
}
