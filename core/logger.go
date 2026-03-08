package core

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func Log(a ...interface{}) {
	msg := fmt.Sprintln(a...)
	logToBoth(msg, 2)
}

func Logf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	logToBoth(msg, 2)
}

func logToBoth(msg string, skip int) {
	fmt.Print(msg)

	if LogFile != nil {
		_, file, _, ok := runtime.Caller(skip)
		source := "unknown"
		if ok {
			file = filepath.ToSlash(file)
			if idx := strings.Index(file, "AutoInstall/"); idx != -1 {
				source = file[idx+len("AutoInstall/"):]
				source = strings.TrimSuffix(source, ".go")
			} else {
				source = filepath.Base(file)
			}
		}

		timestamp := time.Now().Format("2006-01-02 15:04:05")
		cleanMsg := strings.TrimRight(msg, "\n")
		if cleanMsg != "" {
			logLine := fmt.Sprintf("[%s][%s] %s\n", timestamp, source, cleanMsg)
			LogFile.WriteString(logLine)
		}
	}
}

type streamLogWriter struct {
	console io.Writer
	source  string
	buf     bytes.Buffer
	mu      sync.Mutex
}

func newStreamLogWriter(source string, console io.Writer) *streamLogWriter {
	return &streamLogWriter{console: console, source: source}
}

func (w *streamLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n, err := w.console.Write(p)
	if len(p) == 0 {
		return n, err
	}

	_, _ = w.buf.Write(p)
	w.flushCompletedLinesLocked()
	return n, err
}

func (w *streamLogWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.buf.Len() == 0 {
		return
	}
	writeLogLine(w.source, strings.TrimRight(w.buf.String(), "\r\n"))
	w.buf.Reset()
}

func (w *streamLogWriter) flushCompletedLinesLocked() {
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			if line != "" {
				_, _ = w.buf.WriteString(line)
			}
			return
		}
		writeLogLine(w.source, strings.TrimRight(line, "\r\n"))
	}
}

func writeLogLine(source, msg string) {
	if LogFile == nil || msg == "" {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	LogFile.WriteString(fmt.Sprintf("[%s][%s] %s\n", timestamp, source, msg))
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
		timestamp := time.Now().Format("2006-01-02 15:04:05")

		_, file, _, ok := runtime.Caller(1)
		source := "unknown"
		if ok {
			file = filepath.ToSlash(file)
			if idx := strings.Index(file, "AutoInstall/"); idx != -1 {
				source = file[idx+len("AutoInstall/"):]
				source = strings.TrimSuffix(source, ".go")
			} else {
				source = filepath.Base(file)
			}
		}

		msg := fmt.Sprintf("[%s][%s] Error downloading %s: %v | Response: %s\n", timestamp, source, url, err, response)
		LogFile.WriteString(msg)
	}
}

func CloseLogger() {
	if LogFile != nil {
		LogFile.Close()
	}
}
