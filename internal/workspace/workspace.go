package workspace

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Workspace struct {
	Root string
	mu   sync.Mutex
}

func New(root string) (*Workspace, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("安装根目录不是目录: %s", root)
	}
	return &Workspace{Root: root}, nil
}

func SafeJoin(root, name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.ContainsRune(name, 0) || strings.HasPrefix(name, "/") || strings.Contains(name, ":") || filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return "", fmt.Errorf("非法路径: %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part != "" && part != "." && part != ".." {
			if err := validateFilename(part); err != nil {
				return "", err
			}
		}
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径逃逸: %s", name)
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(base, clean)
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("路径逃逸: %s", name)
	}
	current := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("目标路径含符号链接: %s", current)
		}
	}
	return target, nil
}

func Protected(rel string) bool {
	rel = strings.ToLower(filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(rel, "\\", "/")))))
	return rel == "inst.json" || rel == ".autoinst" || strings.HasPrefix(rel, ".autoinst/")
}

func (w *Workspace) Join(rel string) (string, error) {
	if Protected(rel) {
		return "", fmt.Errorf("安装器控制路径不可部署: %s", rel)
	}
	return SafeJoin(w.Root, rel)
}

func Filename(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("无效下载链接: %s", raw)
	}
	p := u.EscapedPath()
	name := p[strings.LastIndex(p, "/")+1:]
	name, err = url.PathUnescape(name)
	if err != nil {
		return "", fmt.Errorf("下载链接没有合法文件名: %s", raw)
	}
	if err := validateFilename(name); err != nil {
		return "", err
	}
	return name, nil
}

func validateFilename(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00<>:\"|?*") || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return fmt.Errorf("非法文件名: %q", name)
	}
	for _, r := range name {
		if r < 32 {
			return fmt.Errorf("文件名含控制字符")
		}
	}
	stem := strings.ToUpper(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " "))
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9') {
		return fmt.Errorf("保留文件名: %s", name)
	}
	return nil
}

func SameContent(a, b string) (bool, error) {
	hash := func(path string) ([32]byte, error) {
		var out [32]byte
		f, err := os.Open(path)
		if err != nil {
			return out, err
		}
		defer f.Close()
		h := sha256.New()
		if _, err = io.Copy(h, f); err != nil {
			return out, err
		}
		copy(out[:], h.Sum(nil))
		return out, nil
	}
	x, err := hash(a)
	if err != nil {
		return false, err
	}
	y, err := hash(b)
	return x == y, err
}

func (w *Workspace) Commit(temp, rel string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	dst, err := w.Join(rel)
	if err != nil {
		return err
	}
	if info, err := os.Stat(dst); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("目标不是普通文件: %s", dst)
		}
		same, err := SameContent(temp, dst)
		if err != nil {
			return err
		}
		if same {
			return nil
		}
		return fmt.Errorf("文件冲突，未覆盖: %s", rel)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	// Recheck after creating parents, before publishing a verified file.
	if _, err = w.Join(rel); err != nil {
		return err
	}
	return os.Rename(temp, dst)
}

func (w *Workspace) DeployTree(ctx context.Context, source string, skip func(string) bool) ([]string, error) {
	type entry struct {
		src, rel string
		mode     os.FileMode
	}
	var entries []entry
	var skipped []string
	err := filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if path == source {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("不支持部署符号链接: %s", path)
		}
		if Protected(rel) || (skip != nil && skip(filepath.ToSlash(rel))) {
			skipped = append(skipped, rel)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		dst, err := w.Join(rel)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("不是普通文件: %s", path)
		}
		if dstInfo, err := os.Stat(dst); err == nil {
			if !dstInfo.Mode().IsRegular() {
				return fmt.Errorf("文件冲突: %s", rel)
			}
			same, err := SameContent(path, dst)
			if err != nil {
				return err
			}
			if !same {
				return fmt.Errorf("文件冲突，未覆盖: %s", rel)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		entries = append(entries, entry{path, rel, info.Mode().Perm()})
		return nil
	})
	if err != nil {
		return skipped, err
	}
	for _, e := range entries {
		if err = ctx.Err(); err != nil {
			return skipped, err
		}
		dir, err := w.TempDir("deploy-")
		if err != nil {
			return skipped, err
		}
		temp := filepath.Join(dir, "payload")
		err = copyFile(e.src, temp, e.mode)
		if err == nil {
			err = w.Commit(temp, e.rel)
		}
		_ = os.RemoveAll(dir)
		if err != nil {
			return skipped, err
		}
	}
	return skipped, nil
}

func (w *Workspace) TempDir(pattern string) (string, error) {
	cache, err := SafeJoin(w.Root, ".autoinst/work")
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(cache, 0700); err != nil {
		return "", err
	}
	return os.MkdirTemp(cache, pattern)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
