package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/autoinst/AutoInstall/core"
	"github.com/autoinst/AutoInstall/internal/app"
	"github.com/autoinst/AutoInstall/internal/download"
	"github.com/autoinst/AutoInstall/internal/model"
	"github.com/autoinst/AutoInstall/internal/workspace"
	"github.com/autoinst/AutoInstall/pkg"
)

var gitversion string

func main() {
	if gitversion == "" {
		gitversion = "NaN"
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	root, err := os.Getwd()
	if err == nil {
		_, err = app.Run(ctx, os.Args[1:], gitversion, root, os.Stdout, app.Dependencies{
			Discover: func(root, input string) (model.Pack, error) {
				p, err := pkg.DiscoverPack(root, input)
				if p == nil {
					return nil, err
				}
				return p, err
			},
			Open: func(root string) (func(), error) {
				if _, err := workspace.SafeJoin(root, ".autoinst/logs"); err != nil {
					return nil, err
				}
				if err := core.SetupLoggerAt(root); err != nil {
					return nil, err
				}
				cache, err := workspace.SafeJoin(root, ".autoinst/cache")
				if err == nil {
					err = os.MkdirAll(cache, 0755)
				}
				if err != nil {
					core.CloseLogger()
					return nil, err
				}
				original := core.HTTPClient
				core.HTTPClient = download.WithContext(ctx, original)
				return func() { core.HTTPClient = original; core.CloseLogger() }, nil
			},
			Install: pkg.Install,
			Log:     func(message string) { core.Log(message) },
		})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "AutoInstall:", err)
		os.Exit(128)
	}
}
