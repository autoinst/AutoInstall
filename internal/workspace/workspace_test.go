package workspace

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func put(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestFilename(t *testing.T) {
	for _, raw := range []string{"https://example.test/BetterGrassify-1.8.8%2Bfabric.jar?token=x#fragment", "https://example.test/BetterGrassify-1.8.8+fabric.jar"} {
		name, err := Filename(raw)
		if err != nil || name != "BetterGrassify-1.8.8+fabric.jar" {
			t.Fatalf("%s => %q, %v", raw, name, err)
		}
	}
	for _, suffix := range []string{"a%2Fb.jar", "a%5Cb.jar", "%00.jar", "CON.jar", "LPT1.jar", "a%3Ab.jar", "a%20", "a.", "", "..", "%01.jar"} {
		if name, err := Filename("https://example.test/" + suffix); err == nil {
			t.Fatalf("accepted %q: %q", suffix, name)
		}
	}
	if _, err := Filename("file:///tmp/x.jar"); err == nil {
		t.Fatal("accepted non-HTTP URL")
	}
}

func TestSafePathsAndControls(t *testing.T) {
	root := t.TempDir()
	w, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"../escape", "/absolute", `C:\escape`, `a\..\..\escape`, "a:b", "", ".", "bad\x00name"} {
		if _, err := SafeJoin(root, rel); err == nil {
			t.Fatalf("accepted %q", rel)
		}
	}
	for _, rel := range []string{"inst.json", "INST.JSON", "config/../inst.json", ".autoinst/cache/payload"} {
		if _, err := w.Join(rel); err == nil {
			t.Fatalf("control path accepted: %q", rel)
		}
	}
	if _, err := w.Join("config/inst.json"); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Logf("symlink case unavailable: %v", err)
	} else if _, err := SafeJoin(root, "link/file"); err == nil {
		t.Fatal("accepted linked parent")
	}
}

func TestConcurrentCommitAndConflict(t *testing.T) {
	root := t.TempDir()
	w, _ := New(root)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		dir, err := w.TempDir("test-")
		if err != nil {
			t.Fatal(err)
		}
		temp := filepath.Join(dir, "payload")
		put(t, temp, "same")
		wg.Add(1)
		go func() { defer wg.Done(); errs <- w.Commit(temp, "mods/same.jar") }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	temp := filepath.Join(t.TempDir(), "different")
	put(t, temp, "different")
	if err := w.Commit(temp, "mods/same.jar"); err == nil {
		t.Fatal("different file overwritten")
	}
	data, _ := os.ReadFile(filepath.Join(root, "mods/same.jar"))
	if string(data) != "same" {
		t.Fatal("destination changed")
	}
}

func TestDeploymentPreflightAndProtectedFiles(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	w, _ := New(root)
	put(t, filepath.Join(root, "z-conflict"), "old")
	put(t, filepath.Join(source, "a-first"), "new")
	put(t, filepath.Join(source, "z-conflict"), "new")
	if _, err := w.DeployTree(context.Background(), source, nil); err == nil {
		t.Fatal("expected conflict")
	}
	if _, err := os.Stat(filepath.Join(root, "a-first")); !os.IsNotExist(err) {
		t.Fatal("deployed before preflight completed")
	}
	os.Remove(filepath.Join(source, "z-conflict"))
	put(t, filepath.Join(source, "inst.json"), "unsafe")
	put(t, filepath.Join(source, ".autoinst/cache/file"), "unsafe")
	skipped, err := w.DeployTree(context.Background(), source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 2 {
		t.Fatalf("skipped: %v", skipped)
	}
	if _, err := os.Stat(filepath.Join(root, "inst.json")); !os.IsNotExist(err) {
		t.Fatal("deployed inst.json")
	}
	if _, err := os.Stat(filepath.Join(root, ".autoinst/cache/file")); !os.IsNotExist(err) {
		t.Fatal("deployed control file")
	}
}
