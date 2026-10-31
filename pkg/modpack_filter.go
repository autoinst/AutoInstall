package pkg

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/autoinst/AutoInstall/core"
	"github.com/autoinst/AutoInstall/internal/workspace"
	"github.com/autoinst/AutoInstall/modcheck"
)

func runInstalledModFilterAt(ctx context.Context, root, bundleName string) {
	if _, err := workspace.SafeJoin(root, ".rubbish"); err != nil {
		core.Log("跳过客户端模组处理，回收目录不安全:", err)
		return
	}
	mods, err := workspace.SafeJoin(root, "mods")
	if err != nil {
		core.Log("跳过客户端模组处理，模组目录不安全:", err)
		return
	}
	moved, err := modcheck.FilterInstalledMods(mods, bundleName, &modcheck.Config{
		Context: ctx, EnableModrinth: true, EnableMixin: true, EnableHash: true,
		Timeout: modcheck.DefaultConfig.Timeout, RubbishDir: filepath.Join(root, ".rubbish"),
	})
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
