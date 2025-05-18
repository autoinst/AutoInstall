package core

import (
	"fmt"
	"os"
)

func Argument(gitversion string) {
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Println("--help 获取帮助")
		fmt.Println("--version 获取版本")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("AutoInstall-" + gitversion)
		return
	}
}
