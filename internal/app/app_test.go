package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autoinst/AutoInstall/internal/model"
)

type fakePack struct {
	prepareErr, closeErr error
	prepares, closes     int
}

func (*fakePack) Target() model.Target {
	return model.Target{Version: "26.3", Loader: "fabric", LoaderVersion: "0.19.5"}
}
func (*fakePack) Name() string { return "test pack" }
func (p *fakePack) Prepare(context.Context, model.InstallRequest) error {
	p.prepares++
	return p.prepareErr
}
func (p *fakePack) Close() error { p.closes++; return p.closeErr }

func TestReadOnlyCommands(t *testing.T) {
	for _, arg := range []string{"--help", "--version"} {
		t.Run(arg, func(t *testing.T) {
			root := t.TempDir()
			var out bytes.Buffer
			fail := func() { t.Fatal("side effect for read-only command") }
			deps := Dependencies{Discover: func(string, string) (model.Pack, error) { fail(); return nil, nil }, Open: func(string) (func(), error) { fail(); return nil, nil }, Install: func(context.Context, model.InstallRequest, bool) error { fail(); return nil }}
			if _, err := Run(context.Background(), []string{arg}, "test-version", root, &out, deps); err != nil {
				t.Fatal(err)
			}
			if out.Len() == 0 {
				t.Fatal("no output")
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 0 {
				t.Fatal("created files")
			}
		})
	}
}

func TestPackLifecycleAndFailures(t *testing.T) {
	boom := errors.New("test failure")
	for _, tc := range []struct {
		name                    string
		prepare, install, close error
		wantInstalls            int
	}{
		{"success", nil, nil, nil, 1}, {"prepare failure", boom, nil, nil, 0}, {"install failure", nil, boom, nil, 1}, {"cleanup failure", nil, nil, boom, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			inst := filepath.Join(root, "inst.json")
			original := `{"version":false,"maxconnections":7,"enableModClassifier":false}`
			if err := os.WriteFile(inst, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			p := &fakePack{prepareErr: tc.prepare, closeErr: tc.close}
			var out bytes.Buffer
			installs, opens, closes := 0, 0, 0
			deps := Dependencies{Discover: func(_ string, input string) (model.Pack, error) {
				if input != "pack.mrpack" {
					t.Fatalf("wrong input %q", input)
				}
				return p, nil
			}, Open: func(string) (func(), error) { opens++; return func() { closes++ }, nil }, Install: func(_ context.Context, r model.InstallRequest, clean bool) error {
				installs++
				if r.Target != p.Target() || r.MaxConnections != 7 || r.EnableModClassifier || clean {
					t.Fatalf("wrong request %+v", r)
				}
				return tc.install
			}}
			_, err := Run(context.Background(), []string{"--modpack=pack.mrpack"}, "test", root, &out, deps)
			failed := tc.prepare != nil || tc.install != nil || tc.close != nil
			if failed != (err != nil) {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Contains(out.String(), "安装完成") == failed {
				t.Fatalf("wrong success log %q", out.String())
			}
			if installs != tc.wantInstalls || opens != 1 || closes != 1 || p.prepares != 1 || p.closes != 1 {
				t.Fatalf("lifecycle mismatch: installs %d opens %d closes %d pack %+v", installs, opens, closes, p)
			}
			data, _ := os.ReadFile(inst)
			if string(data) != original {
				t.Fatal("inst.json modified")
			}
		})
	}
}

func TestExplicitFailureDoesNotFallBack(t *testing.T) {
	root := t.TempDir()
	calls := 0
	deps := Dependencies{Discover: func(_ string, input string) (model.Pack, error) {
		calls++
		if input != "missing" {
			t.Fatal("unexpected fallback")
		}
		return nil, os.ErrNotExist
	}}
	if _, err := Run(context.Background(), []string{"--modpack=missing"}, "test", root, &bytes.Buffer{}, deps); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrong error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("discovered %d times", calls)
	}
}

func TestOrdinaryModeNoDiscoveryOrConfigCreation(t *testing.T) {
	root := t.TempDir()
	installed := false
	deps := Dependencies{Discover: func(string, string) (model.Pack, error) {
		t.Fatal("target flags must bypass discovery")
		return nil, nil
	}, Install: func(_ context.Context, r model.InstallRequest, _ bool) error {
		installed = true
		if r.Version != "1.21.1" || r.Loader != "vanilla" {
			t.Fatalf("wrong target %+v", r.Target)
		}
		return nil
	}}
	if _, err := Run(context.Background(), []string{"--mc-version=1.21.1"}, "test", root, &bytes.Buffer{}, deps); err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("installer not called")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("created config")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	installed = false
	if _, err := Run(ctx, []string{"--mc-version=1.21.1"}, "test", root, &bytes.Buffer{}, deps); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if installed {
		t.Fatal("installed after cancellation")
	}
}
