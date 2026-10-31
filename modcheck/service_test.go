package modcheck

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectionDoesNotMoveAndFilterDoes(t *testing.T) {
	root := t.TempDir()
	mods := filepath.Join(root, "mods")
	if err := os.Mkdir(mods, 0755); err != nil {
		t.Fatal(err)
	}
	jar := filepath.Join(mods, "client.jar")
	f, err := os.Create(jar)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, err := z.Create("client.mixins.json")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(`{"client":["ClientMixin"]}`))
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	cfg := &Config{EnableMixin: true, RubbishDir: filepath.Join(root, "trash")}
	detections, err := DetectInstalledMods(mods, cfg)
	if err != nil || len(detections) != 1 {
		t.Fatalf("detections=%+v error=%v", detections, err)
	}
	if _, err = os.Stat(jar); err != nil {
		t.Fatal("read-only detection moved mod")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg.Context = ctx
	if moved, err := FilterInstalledMods(mods, "pack", cfg); !errors.Is(err, context.Canceled) || len(moved) != 0 {
		t.Fatalf("canceled classification: moved=%v error=%v", moved, err)
	}
	if _, err := os.Stat(jar); err != nil {
		t.Fatal("canceled classification moved mod")
	}
	if _, err := os.Stat(cfg.RubbishDir); !os.IsNotExist(err) {
		t.Fatal("canceled classification created trash directory")
	}
	cfg.Context = context.Background()
	moved, err := FilterInstalledMods(mods, "pack", cfg)
	if err != nil || len(moved) != 1 {
		t.Fatalf("move=%v error=%v", moved, err)
	}
	if _, err = os.Stat(jar); !os.IsNotExist(err) {
		t.Fatal("source retained after move")
	}
	if _, err = os.Stat(filepath.Join(root, "trash/pack/client.jar")); err != nil {
		t.Fatal(err)
	}
}

func TestRepeatedModMovePreservesTrashAndRemovesSource(t *testing.T) {
	for _, content := range []string{"previous client mod", "new client mod"} {
		t.Run(content, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "client.jar")
			target := filepath.Join(root, "trash")
			if err := os.Mkdir(target, 0755); err != nil {
				t.Fatal(err)
			}
			previous := filepath.Join(target, "client.jar")
			if err := os.WriteFile(previous, []byte("previous client mod"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			moved, err := moveMods([]string{source}, target)
			if err != nil || len(moved) != 1 {
				t.Fatalf("moved=%v error=%v", moved, err)
			}
			if _, err := os.Stat(source); !os.IsNotExist(err) {
				t.Fatal("client mod remains in source")
			}
			if data, err := os.ReadFile(previous); err != nil || string(data) != "previous client mod" {
				t.Fatalf("previous trash changed: %q %v", data, err)
			}
			if content != "previous client mod" {
				if data, err := os.ReadFile(filepath.Join(target, "client.1.jar")); err != nil || string(data) != content {
					t.Fatalf("new client mod not preserved: %q %v", data, err)
				}
			}
		})
	}
}

func TestLinkedModsDirectoryIsRejected(t *testing.T) {
	root, external := t.TempDir(), t.TempDir()
	jar := filepath.Join(external, "client.jar")
	if err := os.WriteFile(jar, []byte("external client"), 0600); err != nil {
		t.Fatal(err)
	}
	mods := filepath.Join(root, "mods")
	if err := os.Symlink(external, mods); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := FilterInstalledMods(mods, "pack", &Config{EnableMixin: true, RubbishDir: filepath.Join(root, "trash")}); err == nil {
		t.Fatal("followed linked mods directory")
	}
	if data, err := os.ReadFile(jar); err != nil || string(data) != "external client" {
		t.Fatalf("external mod changed: %q %v", data, err)
	}
}

func TestMoveConflictPreservesBothFiles(t *testing.T) {
	root := t.TempDir()
	src, dst := filepath.Join(root, "source.jar"), filepath.Join(root, "target.jar")
	os.WriteFile(src, []byte("source"), 0600)
	os.WriteFile(dst, []byte("target"), 0600)
	if err := moveFile(src, dst); err == nil {
		t.Fatal("overwrote existing trash file")
	}
	for path, want := range map[string]string{src: "source", dst: "target"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s changed: %q %v", path, got, err)
		}
	}
}
