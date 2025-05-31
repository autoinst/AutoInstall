package core

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// ProgressReader 用于跟踪 io.Reader 的进度
type ProgressReader struct {
	Reader          io.ReadCloser
	Total           int64
	Current         int64
	FilePath        string
	UpdateInterval  int64 // 秒
	lastUpdatedTime int64
}

var gitversion string

// Read 实现 io.Reader 接口
func (pr *ProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.Reader.Read(p)
	pr.Current += int64(n)

	now := time.Now().Unix()
	if now-pr.lastUpdatedTime >= pr.UpdateInterval || err == io.EOF {
		pr.lastUpdatedTime = now
		percent := float64(pr.Current) / float64(pr.Total) * 100
		fmt.Printf("下载进度: %.2f%% (%s)\n", percent, pr.FilePath)
	}
	return
}

// URL 和路径下载文件，支持断点续传和分片下载
func DownloadFile(url, filePath string) error {
	if gitversion == "" {
		gitversion = "NaN"
	}
	const minChunkSize = 10 * 1024 * 1024 // 10MB
	const chunkSize = 2 * 1024 * 1024     // 2MB

	resp, err := http.Head(url)
	if err != nil {
		return fmt.Errorf("无法获取文件信息: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HEAD 请求失败，状态码: %d", resp.StatusCode)
	}

	totalSize := resp.ContentLength
	useChunk := totalSize > minChunkSize && !startsWith(url, "https://bmclapi2.bangbang93.com")

	if useChunk {
		fmt.Println("启用分片下载...")
		return chunkDownload(url, filePath, totalSize, chunkSize)
	}

	return normalDownload(url, filePath, totalSize)
}

// 普通下载 + 断点续传
func normalDownload(url, filePath string, totalSize int64) error {
	const maxRetries = 3

	fileInfo, err := os.Stat(filePath)
	var start int64 = 0
	if err == nil {
		start = fileInfo.Size()
		fmt.Printf("文件已存在，尝试断点续传，已下载大小: %d 字节\n", start)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查文件状态失败: %w", err)
	}

	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			fmt.Printf("创建请求失败 %d/%d: %v\n", i+1, maxRetries, err)
			continue
		}

		if start > 0 {
			req.Header.Set("Range", "bytes="+strconv.FormatInt(start, 10)+"-")
		}
		req.Header.Set("User-Agent", "autoinst/1.1.3")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("尝试 %d/%d 下载失败: %v\n", i+1, maxRetries, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			fmt.Printf("文件未找到，跳过: %s\n", url)
			return nil
		}

		if start > 0 && resp.StatusCode != http.StatusPartialContent {
			fmt.Printf("服务器不支持断点续传，状态码: %d，重新下载...\n", resp.StatusCode)
			start = 0
			os.Remove(filePath)
			continue
		}

		if start == 0 && resp.StatusCode != http.StatusOK {
			fmt.Printf("HTTP 状态码: %d\n", resp.StatusCode)
			continue
		}

		var outFile *os.File
		if start > 0 {
			outFile, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
		} else {
			outFile, err = os.Create(filePath)
		}
		if err != nil {
			return fmt.Errorf("打开文件失败: %w", err)
		}
		defer outFile.Close()

		reader := &ProgressReader{
			Reader:         resp.Body,
			Total:          totalSize,
			Current:        start,
			FilePath:       filePath,
			UpdateInterval: 3,
		}

		_, err = io.Copy(outFile, reader)
		if err != nil {
			fmt.Printf("写入失败 %d/%d: %v\n", i+1, maxRetries, err)
			continue
		}

		fmt.Println("下载完成！")
		return nil
	}

	return fmt.Errorf("多次尝试下载失败")
}

// 分片下载
func chunkDownload(url, filePath string, totalSize int64, chunkSize int64) error {
	outFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer outFile.Close()

	var downloaded int64 = 0
	client := &http.Client{}

	for downloaded < totalSize {
		end := downloaded + chunkSize - 1
		if end >= totalSize {
			end = totalSize - 1
		}

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", downloaded, end))
		req.Header.Set("User-Agent", "autoinst/1.1.3")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("分片请求失败: %w", err)
		}
		if resp.StatusCode != http.StatusPartialContent {
			return fmt.Errorf("服务器不支持分片下载，状态码: %d", resp.StatusCode)
		}

		n, err := io.Copy(outFile, resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("写入分片失败: %w", err)
		}

		downloaded += n
		percent := float64(downloaded) / float64(totalSize) * 100
		fmt.Printf("下载进度: %.2f%% (%s)\n", percent, filePath)
	}

	fmt.Println("下载完成！")
	return nil
}

// 判断字符串前缀
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
