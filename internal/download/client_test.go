package download

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestClientsAreIndependent(t *testing.T) {
	for _, content := range []string{"first", "second"} {
		t.Run(content, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, content) }))
			defer server.Close()
			sum := sha1.Sum([]byte(content))
			path := filepath.Join(t.TempDir(), "part")
			client := Client{HTTP: server.Client()}
			used, err := client.Candidates(context.Background(), []string{server.URL + "/file"}, path, 1, map[string]string{"sha1": hex.EncodeToString(sum[:])})
			if err != nil || used != server.URL+"/file" {
				t.Fatalf("download: %s %v", used, err)
			}
			got, _ := os.ReadFile(path)
			if string(got) != content {
				t.Fatal("clients mixed state")
			}
		})
	}
}
func TestCandidatesRetryHashFailuresWithinSource(t *testing.T) {
	for _, recover := range []bool{false, true} {
		t.Run(fmt.Sprint(recover), func(t *testing.T) {
			calls := 0
			client := Client{HTTP: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Header.Get("Range") != "" {
					t.Error("resumed hash-invalid content")
				}
				body := "corrupt"
				if recover && calls == 2 {
					body = "verified"
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}, nil
			})}}
			sum := sha1.Sum([]byte("verified"))
			path := filepath.Join(t.TempDir(), "part")
			used, err := client.Candidates(context.Background(), []string{"https://example.test/file"}, path, 3, map[string]string{"sha1": hex.EncodeToString(sum[:])})
			if recover {
				if err != nil || calls != 2 || used != "https://example.test/file" {
					t.Fatalf("used=%s calls=%d error=%v", used, calls, err)
				}
			} else if err == nil || calls != 3 {
				t.Fatalf("calls=%d error=%v", calls, err)
			} else if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("failed candidate retained staging")
			}
		})
	}
}

func TestBoundHTTPContextLivesUntilBodyClose(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	var observed context.Context
	original := &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		observed = r.Context()
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data"))}, nil
	})}
	bound := WithContext(parent, original)
	resp, err := bound.Get("https://example.test/file")
	if err != nil {
		t.Fatal(err)
	}
	if observed.Err() != nil {
		t.Fatal("context canceled before body consumed")
	}
	cancel()
	select {
	case <-observed.Done():
	case <-time.After(time.Second):
		t.Fatal("session cancellation not propagated")
	}
	resp.Body.Close()
	if original.Transport == bound.Transport {
		t.Fatal("original client mutated")
	}
}
func TestBodyCloseCancelsBoundRequest(t *testing.T) {
	var observed context.Context
	client := WithContext(context.Background(), &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		observed = r.Context()
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data"))}, nil
	})})
	resp, err := client.Get("https://example.test/file")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if observed.Err() != context.Canceled {
		t.Fatal("request binding leaked after close")
	}
}
