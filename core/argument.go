package core

import (
	"fmt"
	"os"
)

func Argument(gitversion string) bool {
	cleaninst := false
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Println("--help 获取帮助")
		fmt.Println("--version 获取版本")
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("AutoInstall-" + gitversion)
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "--clean" {
		cleaninst = true
	}
	return cleaninst
}
