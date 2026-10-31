package modpack

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/autoinst/AutoInstall/internal/model"
	"github.com/autoinst/AutoInstall/internal/workspace"
)

func executePackFiles(ctx context.Context, w *workspace.Workspace, files []model.FileTask, opts model.Options, deps Dependencies) error {
	var tasks []func() error
	for _, file := range files {
		file := file
		tasks = append(tasks, func() error {
			dir, err := w.TempDir("download-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)
			temp := filepath.Join(dir, "payload.part")
			used, err := deps.Download.Candidates(ctx, file.URLs, temp, opts.MaxRetries, file.Hashes)
			if err != nil {
				return fmt.Errorf("下载 %s: %w", file.Path, err)
			}
			rel := file.Path
			if file.NameFromURL {
				name, err := workspace.Filename(used)
				if err != nil {
					return err
				}
				rel = filepath.Join(filepath.Dir(rel), name)
			}
			if err = w.Commit(temp, rel); err != nil {
				return err
			}
			deps.log("下载完成:", filepath.ToSlash(rel))
			return nil
		})
	}
	return runPackTasks(ctx, opts.MaxConnections, tasks)
}
