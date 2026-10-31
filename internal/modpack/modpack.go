package modpack

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/autoinst/AutoInstall/internal/model"
	"github.com/autoinst/AutoInstall/internal/workspace"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type PackInfo struct {
	kind       string
	path       string
	entry      string
	archive    bool
	name       string
	target     model.Target
	index      *ModrinthIndex
	manifest   *CurseForgeManifest
	workDir    string
	classifier func(context.Context, string, string)
	deps       Dependencies
}

func (p *PackInfo) Target() model.Target { return p.target }
func (p *PackInfo) Name() string         { return p.name }
func (p *PackInfo) Close() error {
	if p.workDir == "" {
		return nil
	}
	err := os.RemoveAll(p.workDir)
	if err == nil {
		p.workDir = ""
	}
	return err
}

// DiscoverPack reads metadata only. It never extracts, downloads or writes configuration.
func DiscoverPack(root, input string, deps Dependencies) (*PackInfo, error) {
	if input == "" {
		var err error
		input, err = detectPackFile(root)
		if err != nil || input == "" {
			return nil, err
		}
	} else if !filepath.IsAbs(input) {
		input = filepath.Join(root, input)
	}
	input, err := filepath.Abs(input)
	if err != nil {
		return nil, err
	}
	p := &PackInfo{path: input, name: bundleNameFromPath(input), deps: deps, classifier: deps.Classify}
	var data []byte
	switch strings.ToLower(filepath.Ext(input)) {
	case ".zip", ".mrpack":
		p.archive = true
		r, err := zip.OpenReader(input)
		if err != nil {
			return nil, fmt.Errorf("打开整合包: %w", err)
		}
		defer r.Close()
		for _, candidate := range []string{"modrinth.index.json", "manifest.json", "variables.txt"} {
			var match *zip.File
			for _, f := range r.File {
				name := zipEntryName(f)
				if filepath.Base(filepath.FromSlash(strings.ReplaceAll(name, "\\", "/"))) != candidate {
					continue
				}
				if match != nil {
					return nil, fmt.Errorf("压缩包内存在多个 %s", candidate)
				}
				if _, err := safeJoin(root, name); err != nil {
					return nil, err
				}
				if f.FileInfo().Mode()&os.ModeSymlink != 0 {
					return nil, fmt.Errorf("metadata 不得为符号链接")
				}
				match = f
				p.entry = name
			}
			if match == nil {
				continue
			}
			data, err = readZipMetadata(match)
			if err != nil {
				return nil, err
			}
			p.kind = candidate
			if candidate == "manifest.json" && manifestIsSPC(data) {
				continue
			}
			break
		}
		if p.kind == "manifest.json" && manifestIsSPC(data) {
			return nil, fmt.Errorf("SPC 压缩包缺少 variables.txt")
		}
		if p.kind == "" {
			return nil, fmt.Errorf("未找到受支持的整合包入口")
		}
	case ".json", ".txt":
		p.kind = strings.ToLower(filepath.Base(input))
		if p.kind != "modrinth.index.json" && p.kind != "manifest.json" && p.kind != "variables.txt" {
			return nil, fmt.Errorf("不支持的整合包清单: %s", input)
		}
		data, err = os.ReadFile(input)
		if err != nil {
			return nil, err
		}
		if p.kind == "manifest.json" && manifestIsSPC(data) {
			p.path = filepath.Join(filepath.Dir(input), "variables.txt")
			p.kind = "variables.txt"
			data, err = os.ReadFile(p.path)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("无法识别的整合包: %s", input)
	}
	switch p.kind {
	case "modrinth.index.json":
		p.index = &ModrinthIndex{}
		if err = json.Unmarshal(data, p.index); err == nil {
			p.target, err = modrinthTarget(*p.index)
		}
	case "manifest.json":
		p.manifest = &CurseForgeManifest{}
		if err = json.Unmarshal(data, p.manifest); err == nil {
			p.target, err = curseForgeTarget(*p.manifest)
		}
	case "variables.txt":
		var vars map[string]string
		vars, err = parseVariables(strings.NewReader(string(data)))
		if err == nil {
			p.target, err = spcTarget(vars)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("解析整合包 %s: %w", input, err)
	}
	if p.target.Version == "" {
		return nil, fmt.Errorf("整合包没有声明 Minecraft 版本")
	}
	return p, nil
}

func detectPackFile(root string) (string, error) {
	mr, err := filepath.Glob(filepath.Join(root, "*.mrpack"))
	if err != nil {
		return "", err
	}
	zips, err := filepath.Glob(filepath.Join(root, "*.zip"))
	if err != nil {
		return "", err
	}
	packs := append(mr, zips...)
	if len(packs) == 1 {
		return packs[0], nil
	}
	if len(packs) > 1 {
		for _, preferred := range []string{"modpack.mrpack", "modpack.zip"} {
			for _, p := range packs {
				if strings.EqualFold(filepath.Base(p), preferred) {
					return p, nil
				}
			}
		}
		return "", fmt.Errorf("发现多个整合包，请使用 --modpack 指定，或命名为 modpack.mrpack / modpack.zip: %s", strings.Join(packs, ", "))
	}
	for _, name := range []string{"variables.txt", "modrinth.index.json", "manifest.json"} {
		p := filepath.Join(root, name)
		info, err := os.Stat(p)
		if err == nil && !info.IsDir() {
			return p, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", nil
}

func (p *PackInfo) Prepare(ctx context.Context, req model.InstallRequest) error {
	if p.workDir != "" {
		return fmt.Errorf("整合包已准备，请先关闭会话")
	}
	w, err := workspace.New(req.Root)
	if err != nil {
		return err
	}
	plan := model.InstallPlan{Target: req.Target}
	switch p.kind {
	case "modrinth.index.json":
		plan.Files, err = planModrinth(w, *p.index, req.Options, p.deps)
	case "manifest.json":
		plan.Files, err = planCurseForge(ctx, *p.manifest, req.Options, p.deps)
	}
	if err != nil {
		return err
	}
	p.workDir, err = w.TempDir("pack-")
	if err != nil {
		return err
	}
	source := filepath.Dir(p.path)
	if p.archive {
		source = filepath.Join(p.workDir, "extracted")
		if err = os.Mkdir(source, 0700); err != nil {
			return err
		}
		if err = extractZip(ctx, p.path, source); err != nil {
			return err
		}
		meta, err := safeJoin(source, p.entry)
		if err != nil {
			return err
		}
		source = filepath.Dir(meta)
	}
	payload := filepath.Join(p.workDir, "payload")
	if err = os.Mkdir(payload, 0700); err != nil {
		return err
	}
	overrides := "overrides"
	if p.manifest != nil && p.manifest.Overrides != "" {
		overrides = p.manifest.Overrides
	}
	if _, err = safeJoin(source, overrides); err != nil {
		return err
	}
	if p.archive || (p.kind == "variables.txt" && filepath.Clean(source) != filepath.Clean(w.Root)) {
		skip := func(rel string) bool {
			norm := filepath.ToSlash(rel)
			if norm == p.kind || norm == filepath.ToSlash(overrides) || strings.HasPrefix(norm, filepath.ToSlash(overrides)+"/") || norm == "server-overrides" || strings.HasPrefix(norm, "server-overrides/") {
				return true
			}
			return p.kind == "variables.txt" && (filepath.Base(norm) == "start.sh" || filepath.Base(norm) == "run.sh")
		}
		if err = overlay(ctx, source, payload, skip); err != nil {
			return err
		}
	}
	if len(plan.Files) > 0 {
		downloads := filepath.Join(p.workDir, "files")
		if err = os.Mkdir(downloads, 0700); err != nil {
			return err
		}
		staging, err := workspace.New(downloads)
		if err != nil {
			return err
		}
		if err = executePackFiles(ctx, staging, plan.Files, req.Options, p.deps); err != nil {
			return err
		}
		if err = overlay(ctx, downloads, payload, workspace.Protected); err != nil {
			return err
		}
	}
	if p.kind != "variables.txt" {
		for _, dir := range []string{overrides, "server-overrides"} {
			if dir == "server-overrides" && p.index == nil {
				continue
			}
			from, err := safeJoin(source, dir)
			if err != nil {
				return err
			}
			if _, err = os.Stat(from); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return err
			}
			if err = overlay(ctx, from, payload, nil); err != nil {
				return err
			}
		}
	}
	skipped, err := w.DeployTree(ctx, payload, nil)
	for _, name := range skipped {
		p.deps.log("跳过安装器控制文件:", name)
	}
	if err != nil {
		return err
	}
	if req.EnableModClassifier {
		filter := p.classifier
		if filter == nil {
			return fmt.Errorf("已开启分类器，但未配置分类服务")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		filter(ctx, w.Root, p.name)
	}
	return nil
}

func runPackTasks(ctx context.Context, limit int, tasks []func() error) error {
	if limit <= 0 {
		return fmt.Errorf("并发数必须为正整数")
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, limit)
	errs := make(chan error, len(tasks)+1)
loop:
	for _, task := range tasks {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			errs <- ctx.Err()
			break loop
		}
		wg.Add(1)
		go func(task func() error) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := task(); err != nil {
				errs <- err
			}
		}(task)
	}
	wg.Wait()
	close(errs)
	var all []error
	for err := range errs {
		all = append(all, err)
	}
	return errors.Join(all...)
}

func overlay(ctx context.Context, source, dest string, skip func(string) bool) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("不是普通目录: %s", source)
	}
	return filepath.WalkDir(source, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if p == source {
			return nil
		}
		rel, err := filepath.Rel(source, p)
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("不支持符号链接: %s", p)
		}
		if skip != nil && skip(filepath.ToSlash(rel)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		out, err := safeJoin(dest, rel)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(out, 0755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("不是普通文件: %s", p)
		}
		if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		f, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, err = io.Copy(f, in)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}

func extractZip(ctx context.Context, archive, dest string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	seen := make(map[string]bool)
	for _, f := range r.File {
		if err = ctx.Err(); err != nil {
			return err
		}
		name := zipEntryName(f)
		key := strings.ToLower(filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(name, "\\", "/")))))
		if seen[key] {
			return fmt.Errorf("压缩包路径重复: %s", name)
		}
		seen[key] = true
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("不支持解压符号链接: %s", name)
		}
		out, err := safeJoin(dest, name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(out, 0755); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		file, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, f.Mode().Perm())
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(file, in)
		closeErr := file.Close()
		in.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func safeJoin(root, name string) (string, error) { return workspace.SafeJoin(root, name) }
func manifestIsSPC(data []byte) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(data, &m) != nil {
		return false
	}
	_, ok := m["serverPackCreatorVersion"]
	return ok
}
func readZipMetadata(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > 16<<20 {
		return nil, fmt.Errorf("metadata 超过 16 MiB")
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, (16<<20)+1))
	if len(b) > 16<<20 {
		return nil, fmt.Errorf("metadata 过大")
	}
	return b, err
}
func bundleNameFromPath(p string) string {
	base := filepath.Base(p)
	switch strings.ToLower(base) {
	case "modrinth.index.json", "manifest.json", "variables.txt":
		return filepath.Base(filepath.Dir(p))
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}
func zipEntryName(f *zip.File) string {
	if f.NonUTF8 {
		if name, ok := tryFixZipName(f.Name); ok {
			return name
		}
	}
	return f.Name
}
func tryFixZipName(name string) (string, bool) {
	raw, err := charmap.CodePage437.NewEncoder().String(name)
	if err != nil {
		return "", false
	}
	if out, err := simplifiedchinese.GB18030.NewDecoder().Bytes([]byte(raw)); err == nil {
		return string(out), true
	}
	if out, err := simplifiedchinese.GBK.NewDecoder().Bytes([]byte(raw)); err == nil {
		return string(out), true
	}
	return "", false
}
