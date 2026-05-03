package core

import (
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var HTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

type ProgressReader struct {
	Reader          io.ReadCloser
	Total           int64
	Current         int64
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
			percent := float64(pr.Current) / float64(pr.Total) * 100
			Logf("下载进度: %.2f%% (%s)\n", percent, pr.FilePath)
		} else {
			Logf("已下载: %d 字节 (%s)\n", pr.Current, pr.FilePath)
		}
	}
	return
}

func DownloadFile(url, filePath string) error {
	return DownloadFileRetry(url, filePath, DefaultMaxRetries)
}

func DownloadFileRetry(url, filePath string, retries int) error {
	retries = NormalizeRetries(retries)
	var lastErr error

	for i := 0; i < retries; i++ {
		if err := downloadFileOnce(url, filePath, i+1, retries); err != nil {
			lastErr = err
			Logf("尝试 %d/%d 下载失败: %v\n", i+1, retries, err)
			continue
		}
		return nil
	}

	return fmt.Errorf("多次尝试下载失败 (共 %d 次): %w", retries, lastErr)
}

func DownloadFileWithFallback(currentURL, officialURL, filePath string, retries int) error {
	currentURL = strings.TrimSpace(currentURL)
	officialURL = strings.TrimSpace(officialURL)
	if currentURL == "" {
		currentURL = officialURL
	}
	if currentURL == "" {
		return fmt.Errorf("下载地址为空")
	}

	Log("使用当前源下载:", currentURL)
	currentErr := DownloadFileRetry(currentURL, filePath, retries)
	if currentErr == nil {
		return nil
	}

	if officialURL == "" || officialURL == currentURL {
		return currentErr
	}

	Log("当前源下载失败，切换官方源:", officialURL)
	officialErr := DownloadFileRetry(officialURL, filePath, retries)
	if officialErr == nil {
		return nil
	}

	return fmt.Errorf("当前源下载失败: %w; 官方源下载失败: %w", currentErr, officialErr)
}

func GetWithRetry(url string, retries int) (*http.Response, error) {
	return GetWithFallback(url, "", retries)
}

func GetWithFallback(currentURL, officialURL string, retries int) (*http.Response, error) {
	currentURL = strings.TrimSpace(currentURL)
	officialURL = strings.TrimSpace(officialURL)
	if currentURL == "" {
		currentURL = officialURL
	}
	if currentURL == "" {
		return nil, fmt.Errorf("请求地址为空")
	}

	resp, err := getWithRetrySingle(currentURL, retries)
	if err == nil {
		return resp, nil
	}

	if officialURL == "" || officialURL == currentURL {
		return nil, err
	}

	Log("当前源请求失败，切换官方源:", officialURL)
	officialResp, officialErr := getWithRetrySingle(officialURL, retries)
	if officialErr == nil {
		return officialResp, nil
	}

	return nil, fmt.Errorf("当前源请求失败: %w; 官方源请求失败: %w", err, officialErr)
}

func getWithRetrySingle(url string, retries int) (*http.Response, error) {
	retries = NormalizeRetries(retries)
	var lastErr error

	for i := 0; i < retries; i++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "autoinst/1.3.0")

		resp, err := HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			Logf("请求失败 %d/%d: %v\n", i+1, retries, err)
			continue
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP 状态码 %d: %s", resp.StatusCode, string(body))
			Logf("请求失败 %d/%d: %v\n", i+1, retries, lastErr)
			continue
		}
		return resp, nil
	}

	return nil, fmt.Errorf("多次请求失败 (共 %d 次): %w", retries, lastErr)
}

func downloadFileOnce(url, filePath string, attempt, retries int) error {
	fileInfo, err := os.Stat(filePath)
	var start int64
	if err == nil {
		start = fileInfo.Size()
		Logf("文件已存在，尝试断点续传，已下载大小: %d 字节\n", start)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查文件状态失败: %w", err)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	if start > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(start, 10)+"-")
	}
	req.Header.Set("User-Agent", "autoinst/1.3.0")

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if start > 0 && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		if isCompleteRange(resp.Header.Get("Content-Range"), start) {
			Log("本地文件已完整，跳过下载:", filePath)
			return nil
		}
		Log("断点续传范围无效，删除本地文件后重新下载")
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除未完成文件失败: %w", err)
		}
		return downloadFileFresh(url, filePath, resp, attempt, retries)
	}

	if start > 0 && resp.StatusCode != http.StatusPartialContent {
		Logf("服务器不支持断点续传，状态码: %d，尝试重新下载\n", resp.StatusCode)
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除未完成文件失败: %w", err)
		}
		return downloadFileFresh(url, filePath, resp, attempt, retries)
	}

	if start == 0 && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP 状态码 %d: %s", resp.StatusCode, string(body))
	}

	total := resp.ContentLength + start
	if total <= 0 {
		Log("无法获取文件大小，将不显示进度")
	}

	var outFile *os.File
	if start > 0 {
		outFile, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	} else {
		outFile, err = os.Create(filePath)
	}
	if err != nil {
		return fmt.Errorf("创建/追加文件失败: %w", err)
	}
	defer outFile.Close()

	reader := &ProgressReader{
		Reader:         resp.Body,
		Total:          total,
		Current:        start,
		FilePath:       filePath,
		UpdateInterval: 3,
	}

	if _, err = io.Copy(outFile, reader); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}

	Log("下载完成！")
	return nil
}

func isCompleteRange(contentRange string, localSize int64) bool {
	contentRange = strings.TrimSpace(contentRange)
	_, totalText, ok := strings.Cut(contentRange, "*/")
	if !ok {
		return false
	}
	total, err := strconv.ParseInt(strings.TrimSpace(totalText), 10, 64)
	return err == nil && total == localSize
}

func downloadFileFresh(url, filePath string, oldResp *http.Response, attempt, retries int) error {
	if oldResp.Body != nil {
		_, _ = io.Copy(io.Discard, oldResp.Body)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "autoinst/1.3.0")

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP 状态码 %d: %s", resp.StatusCode, string(body))
	}

	total := resp.ContentLength
	if total <= 0 {
		Log("无法获取文件大小，将不显示进度")
	}

	outFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer outFile.Close()

	reader := &ProgressReader{
		Reader:         resp.Body,
		Total:          total,
		FilePath:       filePath,
		UpdateInterval: 3,
	}

	if _, err = io.Copy(outFile, reader); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}

	Logf("尝试 %d/%d 下载完成\n", attempt, retries)
	return nil
}

func VerifyFileHash(filePath, algorithm, expected string) error {
	expected = strings.ToLower(strings.TrimSpace(expected))
	if expected == "" {
		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var hasher hash.Hash
	switch strings.ToLower(strings.TrimSpace(algorithm)) {
	case "sha1":
		hasher = sha1.New()
	case "sha512":
		hasher = sha512.New()
	default:
		return fmt.Errorf("不支持的哈希算法: %s", algorithm)
	}

	if _, err := io.Copy(hasher, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if actual != expected {
		return fmt.Errorf("%s 校验失败: expected %s, got %s", algorithm, expected, actual)
	}
	return nil
}
