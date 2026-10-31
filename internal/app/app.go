package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/autoinst/AutoInstall/internal/config"
	"github.com/autoinst/AutoInstall/internal/model"
)

type Dependencies struct {
	Discover func(root, input string) (model.Pack, error)
	Open     func(root string) (func(), error)
	Install  func(context.Context, model.InstallRequest, bool) error
	Log      func(string)
}

func Run(ctx context.Context, args []string, version, root string, out io.Writer, deps Dependencies) (result model.InstallResult, err error) {
	cli, err := config.Parse(args)
	if err != nil {
		return result, err
	}
	if cli.Help {
		config.Help(out)
		return result, nil
	}
	if cli.Version {
		fmt.Fprintln(out, "AutoInstall-"+version)
		return result, nil
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return result, err
	}
	var pack model.Pack
	packClosed := false
	if cli.Modpack != "" || cli.DiscoverAutomatically() {
		if deps.Discover == nil {
			return result, fmt.Errorf("未配置整合包适配器")
		}
		pack, err = deps.Discover(root, cli.Modpack)
		if err != nil {
			return result, err
		}
		if pack != nil {
			defer func() {
				if !packClosed {
					err = errors.Join(err, pack.Close())
				}
			}()
		}
	}
	var target *model.Target
	if pack != nil {
		t := pack.Target()
		target = &t
		result.PackName = pack.Name()
	}
	req, warnings, err := config.Load(filepath.Join(root, "inst.json"), target, cli)
	if err != nil {
		if errors.Is(err, config.ErrMissingTarget) {
			config.Help(out)
		}
		return result, err
	}
	req.Root = root
	result.Target = req.Target
	result.Warnings = warnings
	if deps.Open != nil {
		close, openErr := deps.Open(root)
		if openErr != nil {
			return result, openErr
		}
		if close != nil {
			defer close()
		}
	}
	log := deps.Log
	if log == nil {
		log = func(message string) { fmt.Fprintln(out, message) }
	}
	log("AutoInstall-" + version + " https://github.com/autoinst/AutoInstall")
	log(fmt.Sprintf("安装目标: Minecraft %s, %s %s", req.Version, req.Loader, req.LoaderVersion))
	log(fmt.Sprintf("执行选项: 下载源 %s, 最大并发 %d, 每源最多尝试 %d 次", req.Download, req.MaxConnections, req.MaxRetries))
	for _, warning := range warnings {
		log("[警告] " + warning)
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if pack != nil {
		if err = pack.Prepare(ctx, req); err != nil {
			return result, fmt.Errorf("整合包准备失败: %w", err)
		}
	}
	if deps.Install == nil {
		return result, fmt.Errorf("未配置服务端安装器")
	}
	if err = deps.Install(ctx, req, cli.Clean); err != nil {
		return result, fmt.Errorf("安装失败: %w", err)
	}
	if pack != nil {
		packClosed = true
		if err = pack.Close(); err != nil {
			return result, fmt.Errorf("清理整合包工作目录失败: %w", err)
		}
	}
	log("安装完成：服务端产物和启动脚本已就绪")
	return result, nil
}
