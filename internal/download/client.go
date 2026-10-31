package download

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

const DefaultRetries = 3

func NormalizeRetries(n int) int {
	if n <= 0 {
		return DefaultRetries
	}
	return n
}

type Client struct {
	HTTP     *http.Client
	Logf     func(string, ...any)
	Progress func(current, total int64, path string)
}

func (c Client) log(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}
func UniqueURLs(candidates []string) []string {
	seen := make(map[string]bool)
	var urls []string
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && !seen[candidate] {
			urls = append(urls, candidate)
			seen[candidate] = true
		}
	}
	return urls
}
func RemovePartial(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除下载分片失败: %w", err)
	}
	return nil
}

// File may resume an existing caller-owned partial belonging to this URL only.
func (c Client) File(ctx context.Context, rawURL, path string, retries int) error {
	retries = NormalizeRetries(retries)
	var last error
	transfer := Transfer{Client: c.HTTP, Progress: c.Progress}
	for i := 0; i < retries; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := transfer.Once(ctx, rawURL, path); err != nil {
			last = err
			c.log("尝试 %d/%d 下载失败: %v\n", i+1, retries, err)
			continue
		}
		return nil
	}
	return fmt.Errorf("多次尝试下载失败 (共 %d 次): %w", retries, last)
}

// Candidates accepts a URL only after transfer and every supplied hash succeeds.
// The destination must be caller-owned staging, never a user-owned final file.
func (c Client) Candidates(ctx context.Context, candidates []string, path string, retries int, hashes map[string]string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	urls := UniqueURLs(candidates)
	if len(urls) == 0 {
		return "", fmt.Errorf("下载地址为空")
	}
	for algorithm, expected := range hashes {
		if strings.TrimSpace(expected) == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(algorithm)) {
		case "sha1", "sha512":
		default:
			return "", fmt.Errorf("不支持的哈希算法: %s", algorithm)
		}
	}
	retries = NormalizeRetries(retries)
	transfer := Transfer{Client: c.HTTP, Progress: c.Progress}
	var failures []error
	for i, rawURL := range urls {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := RemovePartial(path); err != nil {
			return "", err
		}
		if i > 0 {
			c.log("当前候选下载或校验失败，切换备用源: %s\n", rawURL)
		}
		var err error
		for attempt := 0; attempt < retries; attempt++ {
			if err = ctx.Err(); err != nil {
				break
			}
			if err = transfer.Once(ctx, rawURL, path); err == nil {
				for algorithm, expected := range hashes {
					if err = VerifyFileHash(path, algorithm, expected); err != nil {
						break
					}
				}
				if err == nil {
					return rawURL, nil
				}
				if cleanup := RemovePartial(path); cleanup != nil {
					return "", errors.Join(err, cleanup)
				}
			}
			c.log("尝试 %d/%d 下载或校验失败: %v\n", attempt+1, retries, err)
		}
		failures = append(failures, fmt.Errorf("候选 %s: %w", rawURL, err))
		if err := RemovePartial(path); err != nil {
			return "", errors.Join(append(failures, err)...)
		}
	}
	return "", fmt.Errorf("所有候选下载或校验失败: %w", errors.Join(failures...))
}

func (c Client) Get(ctx context.Context, candidates []string, retries int) (*http.Response, error) {
	urls := UniqueURLs(candidates)
	if len(urls) == 0 {
		return nil, fmt.Errorf("请求地址为空")
	}
	retries = NormalizeRetries(retries)
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	var failures []error
	for i, rawURL := range urls {
		if i > 0 {
			c.log("当前源请求失败，切换备用源: %s\n", rawURL)
		}
		var last error
		for attempt := 0; attempt < retries; attempt++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("User-Agent", "autoinst/1.4")
			resp, err := client.Do(req)
			if err != nil {
				last = err
				continue
			}
			if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
				last = StatusError(resp)
				resp.Body.Close()
				continue
			}
			return resp, nil
		}
		failures = append(failures, fmt.Errorf("来源 %s: 多次请求失败 (共 %d 次): %w", rawURL, retries, last))
	}
	return nil, errors.Join(failures...)
}
