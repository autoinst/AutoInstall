package main

import "log"

// PublicFunction 是可以被其他文件调用的函数
func PublicFunction() {
	log.Println("这是来自 forge.go 的函数")
}
