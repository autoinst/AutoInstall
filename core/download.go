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
	UpdateInterval  int64 // 更新间隔，单位秒
	lastUpdatedTime int64
}

// Read 实现了 io.Reader 接口
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

func DownloadFile(url, filePath string) error {
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

		client := &http.Client{
			Timeout: 10 * time.Second, // 设置超时
		}
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
			fmt.Printf("服务器不支持断点续传，状态码: %d，尝试重新下载\n", resp.StatusCode)
			start = 0
			os.Remove(filePath) // 删除已存在的文件，重新下载
			continue
		}

		if start == 0 && resp.StatusCode != http.StatusOK {
			fmt.Printf("尝试 %d/%d，HTTP 状态码: %d\n", i+1, maxRetries, resp.StatusCode)
			continue
		}

		total := resp.ContentLength + start
		if total <= 0 {
			fmt.Println("无法获取文件大小，将不显示进度")
		}

		var outFile *os.File
		var createErr error
		if start > 0 {
			outFile, createErr = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
		} else {
			outFile, createErr = os.Create(filePath)
		}

		if createErr != nil {
			return fmt.Errorf("创建/追加文件失败: %w", createErr)
		}
		defer outFile.Close()

		reader := &ProgressReader{
			Reader:         resp.Body,
			Total:          total,
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

	return fmt.Errorf("多次尝试下载失败 (共 %d 次)", maxRetries)
}
