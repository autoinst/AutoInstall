package modpack

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autoinst/AutoInstall/internal/config"
	"github.com/autoinst/AutoInstall/internal/download"
	"github.com/autoinst/AutoInstall/internal/model"
)

func discoverPack(root, input string) (*PackInfo, error) {
	return DiscoverPack(root, input, Dependencies{})
}

const minimalMR = `{"formatVersion":1,"game":"minecraft","versionId":"pack-version","dependencies":{"minecraft":"26.3","fabric-loader":"0.19.5"},"files":[]}`
const minimalCF = `{"minecraft":{"version":"1.21.1","modLoaders":[{"id":"neoforge-21.1.123","primary":true}]},"files":[]}`
const minimalSPC = "# quoted values\nMINECRAFT_VERSION=\"1.21.1\"\nMODLOADER=forge\nMODLOADER_VERSION='52.0.1'\n"

func packFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}
func packZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, data := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(w, data); err != nil {
			t.Fatal(err)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}
func packRequest(root string, p *PackInfo, enabled bool) model.InstallRequest {
	o := config.Defaults()
	o.EnableModClassifier = enabled
	o.MaxRetries = 1
	return model.InstallRequest{Root: root, Target: p.Target(), Options: o}
}

func TestAllFormatsAndClassifierSwitch(t *testing.T) {
	for _, f := range []struct{ name, data string }{{"modrinth.index.json", minimalMR}, {"manifest.json", minimalCF}, {"variables.txt", minimalSPC}, {"manifest.json", strings.Replace(minimalCF, `"files":[]`, `"files":[{"projectID":1,"fileID":2,"required":false}]`, 1)}} {
		for _, archive := range []bool{false, true} {
			for _, enabled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/zip=%v/classifier=%v/%d", f.name, archive, enabled, len(f.data)), func(t *testing.T) {
					root, inputs := t.TempDir(), t.TempDir()
					inst := filepath.Join(root, "inst.json")
					packFile(t, inst, "original config")
					input := filepath.Join(inputs, f.name)
					if archive {
						input = filepath.Join(inputs, "pack.zip")
						packZip(t, input, map[string]string{"nested/" + f.name: f.data, "nested/inst.json": "malicious", "nested/.autoinst/cache/evil": "malicious", "nested/overrides/inst.json": "malicious override", "nested/overrides/config/test.txt": "override", "nested/mods/local.jar": "local"})
					} else {
						packFile(t, input, f.data)
						packFile(t, filepath.Join(inputs, "mods/local.jar"), "local")
						packFile(t, filepath.Join(inputs, "overrides/config/test.txt"), "override")
						packFile(t, filepath.Join(inputs, "overrides/inst.json"), "malicious")
					}
					original, err := os.ReadFile(input)
					if err != nil {
						t.Fatal(err)
					}
					p, err := discoverPack(root, input)
					if err != nil {
						t.Fatal(err)
					}
					calls := 0
					p.classifier = func(_ context.Context, gotRoot, _ string) {
						calls++
						if gotRoot != root {
							t.Fatalf("wrong classifier root %s", gotRoot)
						}
					}
					if err = p.Prepare(context.Background(), packRequest(root, p, enabled)); err != nil {
						t.Fatal(err)
					}
					want := 0
					if enabled {
						want = 1
					}
					if calls != want {
						t.Fatalf("classifier calls=%d", calls)
					}
					owned := p.workDir
					if err = p.Close(); err != nil {
						t.Fatal(err)
					}
					if _, err = os.Stat(owned); !os.IsNotExist(err) {
						t.Fatal("owned directory not cleaned")
					}
					got, _ := os.ReadFile(inst)
					if string(got) != "original config" {
						t.Fatal("inst overwritten")
					}
					after, _ := os.ReadFile(input)
					if !bytes.Equal(after, original) {
						t.Fatal("input changed")
					}
					if _, err = os.Stat(filepath.Join(root, ".autoinst/cache/evil")); !os.IsNotExist(err) {
						t.Fatal("control file deployed")
					}
					if _, err = os.Stat(filepath.Join(root, ".rubbish")); !os.IsNotExist(err) {
						t.Fatal("unexpected mod movement")
					}
				})
			}
		}
	}
}

func TestModrinthOverrideLayeringBeforeUserConflictCheck(t *testing.T) {
	for _, mode := range []string{"fresh", "user conflict", "download failure"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "download failure" {
					io.WriteString(w, "corrupt")
					return
				}
				io.WriteString(w, "downloaded")
			}))
			defer server.Close()
			index := map[string]any{"formatVersion": 1, "game": "minecraft", "dependencies": map[string]string{"minecraft": "1.21.1"}, "files": []any{
				map[string]any{"path": "config/settings.json", "downloads": []string{server.URL + "/settings.json"}, "hashes": map[string]string{"sha512": sha512Text("downloaded")}},
			}}
			encoded, err := json.Marshal(index)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			input := filepath.Join(t.TempDir(), "pack.mrpack")
			packZip(t, input, map[string]string{"modrinth.index.json": string(encoded), "overrides/config/settings.json": "common override", "server-overrides/config/settings.json": "server override"})
			out := filepath.Join(root, "config/settings.json")
			if mode == "user conflict" {
				packFile(t, out, "user configuration")
			}
			p, err := discoverPack(root, input)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			err = p.Prepare(context.Background(), packRequest(root, p, false))
			if (err == nil) != (mode == "fresh") {
				t.Fatalf("mode=%s error=%v", mode, err)
			}
			if mode == "download failure" {
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatal("failed download deployed overrides")
				}
				return
			}
			want := "server override"
			if mode == "user conflict" {
				want = "user configuration"
			}
			if data, err := os.ReadFile(out); err != nil || string(data) != want {
				t.Fatalf("content=%q error=%v want=%q", data, err, want)
			}
		})
	}
}

func TestUnsupportedModrinthLoaderIsRejected(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "modrinth.index.json")
	packFile(t, input, strings.Replace(minimalMR, "fabric-loader", "quilt-loader", 1))
	if _, err := discoverPack(root, input); err == nil || !strings.Contains(err.Error(), "quilt-loader") {
		t.Fatalf("unsupported loader silently accepted: %v", err)
	}
}

func TestPackDiscovery(t *testing.T) {
	root := t.TempDir()
	packFile(t, filepath.Join(root, "version.json"), "{}")
	packFile(t, filepath.Join(root, "install_profile.json"), "{}")
	if p, err := discoverPack(root, ""); err != nil || p != nil {
		t.Fatalf("installer metadata discovered: %v %v", p, err)
	}
	for _, name := range []string{"one.zip", "two.mrpack"} {
		packZip(t, filepath.Join(root, name), map[string]string{"modrinth.index.json": minimalMR})
	}
	if _, err := discoverPack(root, ""); err == nil {
		t.Fatal("ambiguous archives accepted")
	}
	packZip(t, filepath.Join(root, "modpack.zip"), map[string]string{"modrinth.index.json": minimalMR})
	p, err := discoverPack(root, "")
	if err != nil || filepath.Base(p.path) != "modpack.zip" {
		t.Fatalf("preferred archive not used: %v", err)
	}
	packZip(t, filepath.Join(root, "modpack.mrpack"), map[string]string{"modrinth.index.json": minimalMR})
	p, err = discoverPack(root, "")
	if err != nil || filepath.Base(p.path) != "modpack.mrpack" {
		t.Fatalf("mrpack preference lost: %v", err)
	}
	if p.target.Version != "26.3" || p.target.Loader != "fabric" || p.target.LoaderVersion != "0.19.5" {
		t.Fatalf("wrong dependency target %+v", p.target)
	}
	for _, name := range []string{"missing.zip", "version.json"} {
		if _, err = discoverPack(root, name); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
}

func sha512Text(data string) string {
	s := sha512.Sum512([]byte(data))
	return hex.EncodeToString(s[:])
}
func TestModrinthSuccessfulURLNaming(t *testing.T) {
	const data = "verified jar"
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		if strings.HasPrefix(r.URL.Path, "/bad") {
			io.WriteString(w, "bad content")
			return
		}
		io.WriteString(w, data)
	}))
	defer server.Close()
	index := map[string]any{"formatVersion": 1, "game": "minecraft", "dependencies": map[string]string{"minecraft": "26.3", "fabric-loader": "0.19.5"}, "files": []any{
		map[string]any{"path": "mods/nested/declared.jar", "downloads": []string{server.URL + "/bad.jar", server.URL + "/BetterGrassify-1.8.8%2Bfabric.jar?x=1"}, "hashes": map[string]string{"sha512": sha512Text(data), "sha1": "ignored because sha512 exists"}},
		map[string]any{"path": "config/original.json", "downloads": []string{server.URL + "/other.json"}, "hashes": map[string]string{"sha512": sha512Text(data)}},
		map[string]any{"path": "mods/unsupported.jar", "env": map[string]string{"server": "unsupported"}, "downloads": []string{server.URL + "/never.jar"}},
		map[string]any{"path": "inst.json", "downloads": []string{server.URL + "/never.jar"}},
	}}
	encoded, _ := json.Marshal(index)
	for _, follow := range []bool{false, true} {
		t.Run(fmt.Sprint(follow), func(t *testing.T) {
			root := t.TempDir()
			input := filepath.Join(root, "modrinth.index.json")
			packFile(t, input, string(encoded))
			packFile(t, filepath.Join(root, "overrides/mods/override.jar"), "unchanged")
			p, err := discoverPack(root, input)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			p.classifier = func(context.Context, string, string) { t.Fatal("classifier ran while disabled") }
			req := packRequest(root, p, false)
			req.MaxConnections = 1
			req.FollowModpackFilenames = follow
			if err = p.Prepare(context.Background(), req); err != nil {
				t.Fatal(err)
			}
			name := "BetterGrassify-1.8.8+fabric.jar"
			if follow {
				name = "declared.jar"
			}
			got, err := os.ReadFile(filepath.Join(root, "mods/nested", name))
			if err != nil || string(got) != data {
				t.Fatalf("wrong mod content/name: %q, %v", got, err)
			}
			for _, path := range []string{"config/original.json", "mods/override.jar", "modrinth.index.json"} {
				if _, err = os.Stat(filepath.Join(root, path)); err != nil {
					t.Fatal(err)
				}
			}
			for _, path := range []string{"inst.json", "mods/unsupported.jar", ".rubbish"} {
				if _, err = os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
					t.Fatalf("unexpected %s", path)
				}
			}
		})
	}
	if requests["/never.jar"] != 0 {
		t.Fatal("unsupported/protected files downloaded")
	}
	if requests["/bad.jar"] != 2 {
		t.Fatalf("hash fallback not exercised: %+v", requests)
	}
}

func TestUnsafeArchivesDoNotDeploy(t *testing.T) {
	for _, bad := range []string{"../escape.txt", "/absolute.txt", "mods/../../escape.txt", "inst.json.", "mods/CON.jar"} {
		t.Run(bad, func(t *testing.T) {
			root, inputs := t.TempDir(), t.TempDir()
			input := filepath.Join(inputs, "pack.zip")
			packZip(t, input, map[string]string{"modrinth.index.json": minimalMR, bad: "unsafe"})
			p, err := discoverPack(root, input)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			calls := 0
			p.classifier = func(context.Context, string, string) { calls++ }
			if err = p.Prepare(context.Background(), packRequest(root, p, true)); err == nil {
				t.Fatal("accepted unsafe archive")
			}
			if calls != 0 {
				t.Fatal("classified failed pack")
			}
		})
	}
}

func TestInvalidPlanDoesNotDeployOverrides(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "modrinth.index.json")
	broken := strings.Replace(minimalMR, `"files":[]`, `"files":[{"path":"mods/a.jar","downloads":["https://example.test/a.jar"],"hashes":{"sha512":"invalid"}}]`, 1)
	packFile(t, input, broken)
	packFile(t, filepath.Join(root, "overrides/config/test.txt"), "must not deploy")
	p, err := discoverPack(root, input)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err = p.Prepare(context.Background(), packRequest(root, p, false)); err == nil {
		t.Fatal("invalid plan accepted")
	}
	if _, err = os.Stat(filepath.Join(root, "config/test.txt")); !os.IsNotExist(err) {
		t.Fatal("deployed before validation")
	}
	if _, err = os.Stat(filepath.Join(root, "inst.json")); !os.IsNotExist(err) {
		t.Fatal("created inst.json on failure")
	}
	got, _ := os.ReadFile(input)
	if string(got) != broken {
		t.Fatal("changed source metadata on failure")
	}
}

// CF API traffic is intercepted; no real key or external request is needed.
type packTransport func(*http.Request) (*http.Response, error)

func (f packTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCurseForgeRequiredFileAndKeyInjection(t *testing.T) {
	api, downloads := 0, 0
	client := &http.Client{Transport: packTransport(func(r *http.Request) (*http.Response, error) {
		body := "jar"
		if r.URL.Host == "api.curseforge.com" {
			api++
			if r.Header.Get("x-api-key") != "test-only-key" {
				t.Error("missing injected key")
			}
			body = `{"data":"https://files.example.test/Mod%2Bfabric.jar?download=true"}`
		} else if r.URL.Host == "files.example.test" {
			downloads++
		} else {
			t.Errorf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: r}, nil
	})}
	root := t.TempDir()
	input := filepath.Join(root, "manifest.json")
	manifest := strings.Replace(minimalCF, `"files":[]`, `"files":[{"projectID":1,"fileID":2,"required":true},{"projectID":3,"fileID":4,"required":false}]`, 1)
	packFile(t, input, manifest)
	p, err := discoverPack(root, input)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	p.deps.CFAPIKey = "test-only-key"
	p.deps.Download = download.Client{HTTP: client}
	p.classifier = func(context.Context, string, string) { t.Fatal("disabled classifier called") }
	if err = p.Prepare(context.Background(), packRequest(root, p, false)); err != nil {
		t.Fatal(err)
	}
	if api != 1 || downloads != 1 {
		t.Fatalf("requests: api=%d download=%d", api, downloads)
	}
	if got, err := os.ReadFile(filepath.Join(root, "mods/Mod+fabric.jar")); err != nil || string(got) != "jar" {
		t.Fatalf("download missing: %s %v", got, err)
	}
}
