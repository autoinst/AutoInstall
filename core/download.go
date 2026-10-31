package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/autoinst/AutoInstall/internal/download"
)

// HTTPClient remains the compatibility entry point for existing loader adapters.
var HTTPClient = &http.Client{Transport: &http.Transport{
	Proxy:               http.ProxyFromEnvironment,
	DialContext:         (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	TLSHandshakeTimeout: 30 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 90 * time.Second,
}}

func DownloadClient(client *http.Client) download.Client {
	if client == nil {
		client = HTTPClient
	}
	return download.Client{HTTP: client, Logf: Logf, Progress: func(current, total int64, path string) {
		if total > 0 {
			Logf("下载进度: %.2f%% (%s)\n", float64(current)/float64(total)*100, path)
		} else {
			Logf("已下载: %d 字节 (%s)\n", current, path)
		}
	}}
}

// ProgressReader is retained for callers of the legacy core API.
type ProgressReader struct {
	Reader          io.ReadCloser
	Total, Current  int64
	FilePath        string
	UpdateInterval  int64
	lastUpdatedTime int64
}

func (pr *ProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.Reader.Read(p)
	pr.Current += int64(n)
	now := time.Now().Unix()
	if now-pr.lastUpdatedTime >= pr.UpdateInterval || err == io.EOF {
		pr.lastUpdatedTime = now
		if pr.Total > 0 {
			Logf("下载进度: %.2f%% (%s)\n", float64(pr.Current)/float64(pr.Total)*100, pr.FilePath)
		} else {
			Logf("已下载: %d 字节 (%s)\n", pr.Current, pr.FilePath)
		}
	}
	return
}

func DownloadFile(url, path string) error { return DownloadFileRetry(url, path, DefaultMaxRetries) }

// DownloadFileRetry may resume only a caller-owned partial belonging to this URL.
func DownloadFileRetry(url, path string, retries int) error {
	return downloadFileRetry(context.Background(), HTTPClient, url, path, retries)
}
func downloadFileRetry(ctx context.Context, client *http.Client, url, path string, retries int) error {
	return DownloadClient(client).File(ctx, url, path, retries)
}

// DownloadCandidates accepts caller-owned staging and returns the original URL
// only after transfer and all supplied SHA1/SHA512 hashes validate.
func DownloadCandidates(ctx context.Context, client *http.Client, urls []string, path string, retries int, hashes map[string]string) (string, error) {
	return DownloadClient(client).Candidates(ctx, urls, path, retries, hashes)
}

func DownloadFileWithFallback(current, fallback, path string, retries int) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(parent, ".download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	staged := filepath.Join(dir, "payload.part")
	if _, err := DownloadCandidates(context.Background(), HTTPClient, []string{current, fallback}, staged, retries, nil); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("下载目标不是普通文件: %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(staged, path)
}

func GetWithRetry(url string, retries int) (*http.Response, error) {
	return GetWithFallback(url, "", retries)
}
func GetWithFallback(current, fallback string, retries int) (*http.Response, error) {
	return DownloadClient(HTTPClient).Get(context.Background(), []string{current, fallback}, retries)
}
func VerifyFileHash(path, algorithm, expected string) error {
	return download.VerifyFileHash(path, algorithm, expected)
}
