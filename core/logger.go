package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type DownloadError struct {
	URL      string
	Err      error
	Response string
}

var (
	LogFile        *os.File
	DownloadErrors []DownloadError
	ErrorMutex     sync.Mutex
)

func SetupLogger() error {
	logDir := filepath.Join(".autoinst", "logs")
	if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
		return err
	}
	dateStr := time.Now().Format("2006-01-02")
	logPath := filepath.Join(logDir, dateStr+".log")

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	LogFile = f
	return nil
}

func RecordError(url string, err error, response string) {
	ErrorMutex.Lock()
	defer ErrorMutex.Unlock()
	DownloadErrors = append(DownloadErrors, DownloadError{
		URL:      url,
		Err:      err,
		Response: response,
	})

	if LogFile != nil {
		timestamp := time.Now().Format("15:04:05")
		msg := fmt.Sprintf("[%s] Error downloading %s: %v | Response: %s\n", timestamp, url, err, response)
		LogFile.WriteString(msg)
	}
}

func CloseLogger() {
	if LogFile != nil {
		LogFile.Close()
	}
}
