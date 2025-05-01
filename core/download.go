package core

import (
	"fmt"
	"io"
	"net/http"
	"os"
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

	for i := 0; i < maxRetries; i++ {
		resp, err := http.Get(url)
		if err != nil {
			fmt.Printf("尝试 %d/%d 下载失败: %v\n", i+1, maxRetries, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			fmt.Printf("文件未找到，跳过: %s\n", url)
			return nil
		}
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("尝试 %d/%d，HTTP 状态码: %d\n", i+1, maxRetries, resp.StatusCode)
			continue
		}

		// 直接用 ContentLength，省去第二次请求
		total := resp.ContentLength
		if total <= 0 {
			fmt.Println("无法获取文件大小，将不显示进度")
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
			fmt.Printf("写入失败 %d/%d: %v\n", i+1, maxRetries, err)
			continue
		}
		fmt.Println("下载完成！")
		return nil
	}

	return fmt.Errorf("多次尝试下载失败 (共 %d 次)", maxRetries)
}
