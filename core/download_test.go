package core

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type downloadTestTransport func(*http.Request) (*http.Response, error)

func (f downloadTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func downloadTestResponse(status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}

func testDownloadClient(t *testing.T, client *http.Client) {
	t.Helper()
	old := HTTPClient
	HTTPClient = client
	t.Cleanup(func() { HTTPClient = old })
}

func assertDownloadContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("content = %q, %v; want %q", got, err, want)
	}
}

func TestFallbackPreservesDestinationUntilSuccess(t *testing.T) {
	for _, succeed := range []bool{false, true} {
		t.Run(strconv.FormatBool(succeed), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "library.jar")
			if err := os.WriteFile(path, []byte("existing library"), 0600); err != nil {
				t.Fatal(err)
			}
			testDownloadClient(t, &http.Client{Transport: downloadTestTransport(func(r *http.Request) (*http.Response, error) {
				assertDownloadContent(t, path, "existing library")
				if r.Header.Get("Range") != "" {
					t.Error("resumed an existing final destination")
				}
				if succeed && r.URL.Host == "backup.invalid" {
					return downloadTestResponse(http.StatusOK, "new library", nil), nil
				}
				return downloadTestResponse(http.StatusNotFound, "missing", nil), nil
			})})
			err := DownloadFileWithFallback("https://primary.invalid/lib", "https://backup.invalid/lib", path, 1)
			if (err == nil) != succeed {
				t.Fatalf("success=%v error=%v", succeed, err)
			}
			want := "existing library"
			if succeed {
				want = "new library"
			}
			assertDownloadContent(t, path, want)
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 1 {
				t.Fatalf("staging not cleaned: %v %v", entries, err)
			}
		})
	}
}

func TestDownloadRangeResponses(t *testing.T) {
	for _, tc := range []struct {
		name, contentRange, body string
		status                   int
		wantErr                  bool
	}{
		{"resume", "bytes 3-5/6", "def", 206, false},
		{"wrong start", "bytes 2-4/6", "def", 206, true},
		{"wrong end", "bytes 3-6/6", "def", 206, true},
		{"wrong length", "bytes 3-4/6", "def", 206, true},
		{"unknown total", "bytes 3-5/*", "def", 206, true},
		{"ignored range", "", "abcdef", 200, false},
		{"404 remains failure", "", "not found", 404, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "part")
			if err := os.WriteFile(path, []byte("abc"), 0600); err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Transport: downloadTestTransport(func(r *http.Request) (*http.Response, error) {
				if got := r.Header.Get("Range"); got != "bytes=3-" {
					t.Errorf("Range = %q", got)
				}
				return downloadTestResponse(tc.status, tc.body, http.Header{"Content-Range": {tc.contentRange}}), nil
			})}
			err := downloadFileRetry(context.Background(), client, "https://test.invalid/file", path, 1)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				assertDownloadContent(t, path, "abcdef")
			} else {
				assertDownloadContent(t, path, "abc")
			}
		})
	}
}

func TestDownload416RequiresFreshContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "part")
	if err := os.WriteFile(path, []byte("wrong!"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &http.Client{Transport: downloadTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return downloadTestResponse(416, "", http.Header{"Content-Range": {"bytes */6"}}), nil
		}
		if r.Header.Get("Range") != "" {
			t.Error("fresh request must not contain Range")
		}
		return downloadTestResponse(200, "abcdef", nil), nil
	})}
	if err := downloadFileRetry(context.Background(), client, "https://test.invalid/file", path, 1); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
	assertDownloadContent(t, path, "abcdef")
}

type brokenDownloadBody struct{ read bool }

func (b *brokenDownloadBody) Read(p []byte) (int, error) {
	if b.read {
		return 0, io.ErrUnexpectedEOF
	}
	b.read = true
	return copy(p, "abc"), io.ErrUnexpectedEOF
}
func (*brokenDownloadBody) Close() error { return nil }

func TestDownloadRetryResumesOnlySameSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "part")
	calls := 0
	client := &http.Client{Transport: downloadTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: 6, Body: &brokenDownloadBody{}}, nil
		}
		if r.Header.Get("Range") != "bytes=3-" {
			t.Errorf("retry Range = %q", r.Header.Get("Range"))
		}
		return downloadTestResponse(206, "def", http.Header{"Content-Range": {"bytes 3-5/6"}}), nil
	})}
	if err := downloadFileRetry(context.Background(), client, "https://test.invalid/file", path, 2); err != nil {
		t.Fatal(err)
	}
	assertDownloadContent(t, path, "abcdef")
}

func TestDownloadFallbackClearsOldSourcePartial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "part")
	client := &http.Client{Transport: downloadTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "first.invalid" {
			return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: 6, Body: &brokenDownloadBody{}}, nil
		}
		if r.Header.Get("Range") != "" {
			t.Fatal("fallback reused first source partial")
		}
		return downloadTestResponse(200, "uvwxyz", nil), nil
	})}
	testDownloadClient(t, client)
	if err := DownloadFileWithFallback("https://first.invalid/file", "https://second.invalid/file", path, 1); err != nil {
		t.Fatal(err)
	}
	assertDownloadContent(t, path, "uvwxyz")
}

func TestDownloadCandidatesVerifiesBothHashesAndReturnsOriginalURL(t *testing.T) {
	calls := make(map[string]int)
	var callsMu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callsMu.Lock()
		calls[r.URL.Path]++
		callsMu.Unlock()
		if r.Header.Get("Range") != "" {
			t.Error("candidate inherited partial content")
		}
		switch r.URL.Path {
		case "/bad":
			fmt.Fprint(w, "bad!")
		case "/candidate.jar":
			http.Redirect(w, r, "/actual", 302)
		default:
			fmt.Fprint(w, "good")
		}
	}))
	defer srv.Close()
	sha1Sum, sha512Sum := sha1.Sum([]byte("good")), sha512.Sum512([]byte("good"))
	path := filepath.Join(t.TempDir(), "stage")
	url, err := DownloadCandidates(context.Background(), srv.Client(), []string{srv.URL + "/bad", srv.URL + "/bad", srv.URL + "/candidate.jar"}, path, 1, map[string]string{"sha1": hex.EncodeToString(sha1Sum[:]), "sha512": hex.EncodeToString(sha512Sum[:])})
	if err != nil {
		t.Fatal(err)
	}
	callsMu.Lock()
	if url != srv.URL+"/candidate.jar" || calls["/bad"] != 1 {
		t.Errorf("URL = %q; calls = %v", url, calls)
	}
	callsMu.Unlock()
	assertDownloadContent(t, path, "good")
}

func TestDownloadCancellationAndHashFailureCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stage")
	calls := 0
	client := &http.Client{Transport: downloadTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return downloadTestResponse(200, "bad", nil), nil
	})}
	_, err := DownloadCandidates(context.Background(), client, []string{"https://test.invalid/file"}, path, 1, map[string]string{"sha1": strings.Repeat("0", 40)})
	if err == nil {
		t.Fatal("hash mismatch succeeded")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed candidate left stage: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = DownloadCandidates(ctx, client, []string{"https://test.invalid/file"}, path, 3, nil)
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancel error = %v, calls = %d", err, calls)
	}
}

type canceledDownloadBody struct{ cancel context.CancelFunc }

func (b *canceledDownloadBody) Read(p []byte) (int, error) {
	b.cancel()
	return copy(p, "abc"), context.Canceled
}
func (*canceledDownloadBody) Close() error { return nil }

func TestDownloadCancellationDuringBodyCleansPartial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stage")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	client := &http.Client{Transport: downloadTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: 6, Body: &canceledDownloadBody{cancel: cancel}}, nil
	})}
	_, err := DownloadCandidates(ctx, client, []string{"https://first.invalid/file", "https://second.invalid/file"}, path, 3, nil)
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("error = %v; calls = %d", err, calls)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cancellation left a partial file: %v", err)
	}
}

func TestDownloadShort206ResumesUntilTotal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stage")
	if err := os.WriteFile(path, []byte("ab"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &http.Client{Transport: downloadTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return downloadTestResponse(206, "cd", http.Header{"Content-Range": {"bytes 2-3/6"}}), nil
		}
		if r.Header.Get("Range") != "bytes=4-" {
			t.Errorf("Range = %q", r.Header.Get("Range"))
		}
		return downloadTestResponse(206, "ef", http.Header{"Content-Range": {"bytes 4-5/6"}}), nil
	})}
	if err := downloadFileRetry(context.Background(), client, "https://test.invalid/file", path, 2); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("incomplete range accepted: calls %d", calls)
	}
	assertDownloadContent(t, path, "abcdef")
}

func TestDownloadFresh206And416CannotSucceed(t *testing.T) {
	for _, status := range []int{206, 416} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "stage")
			client := &http.Client{Transport: downloadTestTransport(func(r *http.Request) (*http.Response, error) {
				return downloadTestResponse(status, "body", http.Header{"Content-Range": {"bytes 0-3/4"}}), nil
			})}
			if err := downloadFileRetry(context.Background(), client, "https://test.invalid/file", path, 1); err == nil {
				t.Fatalf("fresh %d unexpectedly succeeded", status)
			}
		})
	}
}

func TestDownloadDedup404AndNoCompletionLog(t *testing.T) {
	log, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatal(err)
	}
	oldLog := LogFile
	LogFile = log
	t.Cleanup(func() { LogFile = oldLog; log.Close() })
	calls := 0
	client := &http.Client{Transport: downloadTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return downloadTestResponse(404, "missing", nil), nil
	})}
	testDownloadClient(t, client)
	url := "https://test.invalid/missing"
	if err := DownloadFileWithFallback(url, url, filepath.Join(t.TempDir(), "stage"), 2); err == nil {
		t.Fatal("404 ignored")
	}
	if calls != 2 {
		t.Fatalf("duplicate candidate was retried: %d calls", calls)
	}
	if _, err := GetWithFallback(url, url, 2); err == nil {
		t.Fatal("GET 404 ignored")
	}
	if calls != 4 {
		t.Fatalf("duplicate GET candidate was retried: %d calls", calls)
	}
	client.Transport = downloadTestTransport(func(r *http.Request) (*http.Response, error) { return downloadTestResponse(200, "ok", nil), nil })
	if err := DownloadFileRetry(url, filepath.Join(t.TempDir(), "stage"), 1); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log.Name())
	if err != nil || strings.Contains(string(data), "下载完成") {
		t.Fatalf("low-level completion log: %q, %v", data, err)
	}
}
