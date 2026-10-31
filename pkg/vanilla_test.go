package pkg

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autoinst/AutoInstall/core"
)

func TestServerJarReplacementPreservesExistingUntilVerified(t *testing.T) {
	for _, mode := range []string{"cached", "unavailable", "corrupt", "replacement"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "server.jar")
			if err := os.WriteFile(path, []byte("existing server"), 0600); err != nil {
				t.Fatal(err)
			}
			want := "new server"
			if mode == "cached" {
				want = "existing server"
			}
			sum := sha1.Sum([]byte(want))
			old := core.HTTPClient
			t.Cleanup(func() { core.HTTPClient = old })
			calls := 0
			core.HTTPClient = &http.Client{Transport: libraryTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if data, err := os.ReadFile(path); err != nil || string(data) != "existing server" {
					t.Errorf("existing server changed during download: %q %v", data, err)
				}
				if r.Header.Get("Range") != "" {
					t.Error("resumed final server.jar")
				}
				status, body := 200, "new server"
				if mode == "unavailable" {
					status, body = 404, "missing"
				} else if mode == "corrupt" {
					body = "corrupt"
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}, nil
			})}
			err := downloadServerJarFromURL("https://example.test/server.jar", path, hex.EncodeToString(sum[:]), 1)
			success := mode == "cached" || mode == "replacement"
			if (err == nil) != success {
				t.Fatalf("mode=%s error=%v", mode, err)
			}
			if mode == "cached" && calls != 0 {
				t.Fatal("redownloaded verified cached server")
			}
			if mode != "replacement" {
				want = "existing server"
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != want {
				t.Fatalf("final server=%q error=%v want=%q", data, err, want)
			}
			if entries, err := os.ReadDir(root); err != nil || len(entries) != 1 {
				t.Fatalf("staging not cleaned: %v %v", entries, err)
			}
		})
	}
}

func TestServerMetadataRespectsSourceAndFallback(t *testing.T) {
	for _, source := range []string{"bmclapi", "official"} {
		t.Run(source, func(t *testing.T) {
			old := core.HTTPClient
			t.Cleanup(func() { core.HTTPClient = old })
			officialCalls := 0
			core.HTTPClient = &http.Client{Transport: libraryTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "bmclapi2.bangbang93.com" {
					officialCalls++
					return nil, fmt.Errorf("official host unavailable")
				}
				body := `{"versions":[{"id":"1.21.1","url":"https://piston-meta.mojang.com/version.json"}]}`
				if r.URL.Path == "/version/1.21.1/json" {
					body = `{"downloads":{"server":{"url":"https://example.test/server.jar","sha1":"a9993e364706816aba3e25717850c26c9cd0d89d"}}}`
				} else if r.URL.Path != "/mc/game/version_manifest.json" {
					t.Fatalf("unexpected mirror URL: %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}, nil
			})}
			url, hash, err := getOfficialServerJarURL("1.21.1", 1, source)
			if err != nil || url != "https://example.test/server.jar" || hash == "" {
				t.Fatalf("URL=%s hash=%s error=%v", url, hash, err)
			}
			if source == "bmclapi" && officialCalls != 0 {
				t.Fatalf("mirror mode required official hosts: %d calls", officialCalls)
			}
			if source == "official" && officialCalls != 2 {
				t.Fatalf("official fallback calls=%d", officialCalls)
			}
		})
	}
}

func TestServerDownloadRejectsNestedLibrarySymlink(t *testing.T) {
	t.Chdir(t.TempDir())
	external := t.TempDir()
	if err := os.MkdirAll("libraries/net/minecraft/server", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, "libraries/net/minecraft/server/1.21.1"); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	old := core.HTTPClient
	t.Cleanup(func() { core.HTTPClient = old })
	core.HTTPClient = &http.Client{Transport: libraryTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatal("unsafe destination reached HTTP requests")
		return nil, fmt.Errorf("unexpected request")
	})}
	if err := DownloadServerJar("1.21.1", "neoforge", "libraries", "official", 1, ""); err == nil {
		t.Fatal("accepted nested linked destination")
	}
	if entries, err := os.ReadDir(external); err != nil || len(entries) != 0 {
		t.Fatalf("external directory changed: %v %v", entries, err)
	}
}
