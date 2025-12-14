package core

import (
	"os"
)

func Argument(gitversion string) bool {
	cleaninst := false
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		Log("--help 获取帮助")
		Log("--version 获取版本")
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		Log("AutoInstall-" + gitversion)
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "--clean" {
		cleaninst = true
	}
	return cleaninst
}
