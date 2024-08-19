package main

import (
    "fmt";
    "os";
    "bufio"
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
   text = text[:len(text)-1]
   if text == 0 {
      fmt.Printf("test0\n");
	} else if start == "1"{
      fmt.Printf("test1\n");
	} else {
      fmt.Printf("?你在干啥\n");
      os.Exit(0)
   }
}
