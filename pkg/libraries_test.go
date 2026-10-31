package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/autoinst/AutoInstall/core"
)

type libraryTransport func(*http.Request) (*http.Response, error)

func (f libraryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLibraryHashFailureTriesBackupBeforeCommit(t *testing.T) {
	old := core.HTTPClient
	t.Cleanup(func() { core.HTTPClient = old })
	requests := make(map[string]int)
	core.HTTPClient = &http.Client{Transport: libraryTransport(func(r *http.Request) (*http.Response, error) {
		requests[r.URL.Host]++
		body := "wrong"
		if r.URL.Host == "bmclapi2.bangbang93.com" {
			body = "abc"
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}, nil
	})}
	lib := core.Library{Name: "g:a:1"}
	lib.Downloads.Artifact.URL = "https://maven.minecraftforge.net/g/a/1/a-1.jar"
	lib.Downloads.Artifact.SHA1 = "a9993e364706816aba3e25717850c26c9cd0d89d"
	root := t.TempDir()
	cached := filepath.Join(root, "g/a/1/a-1.jar")
	if err := os.MkdirAll(filepath.Dir(cached), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cached, []byte("stale cache"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := DownloadLibraries(core.VersionInfo{Libraries: []core.Library{lib}}, root, 1, "official", 1); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "g/a/1/a-1.jar"))
	if err != nil || string(got) != "abc" {
		t.Fatalf("invalid committed file: %q %v", got, err)
	}
	if requests["maven.minecraftforge.net"] != 1 || requests["bmclapi2.bangbang93.com"] != 1 {
		t.Fatalf("backup not used: %v", requests)
	}
}

func TestResolveLibraryArtifact(t *testing.T) {
	tests := []struct{ name, raw, url, path string }{
		{"legacy default", `{"name":"a.b:tool:1"}`, "https://libraries.minecraft.net/a/b/tool/1/tool-1.jar", "a/b/tool/1/tool-1.jar"},
		{"legacy repository", `{"name":"a.b:tool:1:server","url":"https://example.invalid/maven"}`, "https://example.invalid/maven/a/b/tool/1/tool-1-server.jar", "a/b/tool/1/tool-1-server.jar"},
		{"legacy file", `{"name":"a.b:tool:1","filePath":"custom.jar"}`, "https://libraries.minecraft.net/a/b/tool/1/custom.jar", "a/b/tool/1/custom.jar"},
		{"modern remote", `{"name":"a.b:tool:1","downloads":{"artifact":{"url":"https://example.invalid/direct.jar","path":"different/tool.jar"}}}`, "https://example.invalid/direct.jar", "different/tool.jar"},
		{"modern empty", `{"name":"a.b:tool:1:client","downloads":{"artifact":{"url":""}}}`, "", "a/b/tool/1/tool-1-client.jar"},
		{"modern absent", `{"name":"a.b:tool:1","downloads":{"artifact":{"path":"a/b/tool.jar"}}}`, "", "a/b/tool.jar"},
		{"modern absent artifact", `{"name":"a.b:tool:1","url":"https://example.invalid/maven","downloads":{}}`, "", "a/b/tool/1/tool-1.jar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lib core.Library
			if err := json.Unmarshal([]byte(tt.raw), &lib); err != nil {
				t.Fatal(err)
			}
			url, path, _ := resolveLibraryArtifact(lib)
			if url != tt.url || path != tt.path {
				t.Fatalf("got %q %q, want %q %q", url, path, tt.url, tt.path)
			}
		})
	}
}

func TestServerLibraryPrefetchDoesNotRequestClientGeneratedOrEmbedded(t *testing.T) {
	var mu sync.Mutex
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.Path]++
		mu.Unlock()
		if r.URL.Path == "/required.jar" || r.URL.Path == "/shared-client.jar" {
			_, _ = w.Write([]byte("abc"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	var info core.VersionInfo
	raw := fmt.Sprintf(`{"libraries":[
 {"name":"g:required:1","downloads":{"artifact":{"url":%q,"sha1":"a9993e364706816aba3e25717850c26c9cd0d89d"}}},
 {"name":"g:shared-client:1","downloads":{"artifact":{"url":%q}}},
 {"name":"net.minecraftforge:forge:26.2-65.1.5:client","downloads":{"artifact":{"url":"","path":"net/minecraftforge/forge/26.2-65.1.5/forge-26.2-65.1.5-client.jar"}}},
 {"name":"g:server-output:1","downloads":{"artifact":{"url":""}}},
 {"name":"g:embedded:1","downloads":{"artifact":{"url":""}}},
 {"name":"g:unknown:1","downloads":{"artifact":{"url":""}}}
 ],"data":{"PATCHED":{"client":"[net.minecraftforge:forge:26.2-65.1.5:client]","server":"[g:server-output:1]"}},
 "processors":[{"sides":["client"],"outputs":{"{PATCHED}":""}},{"sides":["server"],"outputs":{"{PATCHED}":""}}]}`, server.URL+"/required.jar", server.URL+"/shared-client.jar")
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		t.Fatal(err)
	}
	info.EmbeddedFiles = []string{"maven/g/embedded/1/embedded-1.jar"}
	root := t.TempDir()
	if err := DownloadLibraries(info, root, 3, "official", 1); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 || requests["/required.jar"] != 1 || requests["/shared-client.jar"] != 1 {
		t.Fatalf("unexpected requests: %+v", requests)
	}
	for _, file := range []string{"g/required/1/required-1.jar", "g/shared-client/1/shared-client-1.jar"} {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil || string(got) != "abc" {
			t.Fatalf("%s: %q %v", file, got, err)
		}
	}
}

func TestRequiredLibrary404AndHashMismatchAreErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad-hash" {
			_, _ = w.Write([]byte("wrong"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	for _, suffix := range []string{"/missing", "/bad-hash"} {
		t.Run(suffix, func(t *testing.T) {
			lib := core.Library{Name: "g:required:1"}
			lib.Downloads.Artifact.URL = server.URL + suffix
			lib.Downloads.Artifact.SHA1 = "a9993e364706816aba3e25717850c26c9cd0d89d"
			root := t.TempDir()
			if err := DownloadLibraries(core.VersionInfo{Libraries: []core.Library{lib}}, root, 2, "official", 1); err == nil {
				t.Fatal("required remote failure ignored")
			}
			if suffix == "/bad-hash" {
				if _, err := os.Stat(filepath.Join(root, "g", "required", "1", "required-1.jar")); !os.IsNotExist(err) {
					t.Fatalf("corrupt file retained: %v", err)
				}
			}
		})
	}
}

func TestLibraryDuplicatePathSHA1Conflict(t *testing.T) {
	first := core.Library{Name: "g:a:1"}
	first.Downloads.Artifact.URL = "http://127.0.0.1:1/unreachable"
	first.Downloads.Artifact.SHA1 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	second := first
	second.Downloads.Artifact.SHA1 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	err := DownloadLibraries(core.VersionInfo{Libraries: []core.Library{first, second}}, t.TempDir(), 2, "official", 1)
	if err == nil || !strings.Contains(err.Error(), "SHA1") {
		t.Fatalf("conflict ignored: %v", err)
	}
}

func TestLibraryDuplicatesDownloadOnceAndReuseVerifiedFile(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		_, _ = w.Write([]byte("abc"))
	}))
	defer server.Close()
	lib := core.Library{Name: "g:a:1"}
	lib.Downloads.Artifact.URL = server.URL + "/a.jar"
	lib.Downloads.Artifact.SHA1 = "a9993e364706816aba3e25717850c26c9cd0d89d"
	info := core.VersionInfo{Libraries: []core.Library{lib, lib}}
	root := t.TempDir()
	for i := 0; i < 2; i++ {
		if err := DownloadLibraries(info, root, 4, "official", 1); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != 1 {
		t.Fatalf("duplicate downloads = %d", requests)
	}
}
