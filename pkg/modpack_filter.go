package pkg

import (
	"strings"

	"github.com/autoinst/AutoInstall/core"
	"github.com/autoinst/AutoInstall/modcheck"
)

func runInstalledModFilter(bundleName string) {
	moved, err := modcheck.FilterInstalledMods("mods", bundleName, nil)
	if err != nil {
		core.Log("客户端模组检测完成，但存在文件处理错误:", err)
	}
	if len(moved) == 0 {
		return
	}
	bundleLabel := strings.TrimSpace(bundleName)
	if bundleLabel == "" {
		bundleLabel = "modpack"
	}
	core.Logf("客户端模组已移动到 .rubbish/%s\n", bundleLabel)
}
