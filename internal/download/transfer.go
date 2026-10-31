package download

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Transfer owns no global client, logger or destination directory.
type Transfer struct {
	Client   *http.Client
	Progress func(current, total int64, path string)
}

type progressReader struct {
	body           io.Reader
	current, total int64
	path           string
	last           time.Time
	report         func(int64, int64, string)
}

func (p *progressReader) Read(data []byte) (int, error) {
	n, err := p.body.Read(data)
	p.current += int64(n)
	if p.report != nil && (time.Since(p.last) >= 3*time.Second || err == io.EOF) {
		p.last = time.Now()
		p.report(p.current, p.total, p.path)
	}
	return n, err
}

func (t Transfer) Once(ctx context.Context, rawURL, path string) error {
	return t.request(ctx, rawURL, path, true)
}
func (t Transfer) request(ctx context.Context, rawURL, path string, allowRange bool) error {
	var start int64
	if info, err := os.Stat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("下载路径不是普通文件: %s", path)
		}
		if allowRange {
			start = info.Size()
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查文件状态失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if start > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(start, 10)+"-")
	}
	req.Header.Set("User-Agent", "autoinst/1.4")
	// Range offsets describe representation bytes, not transparent gzip output.
	req.Header.Set("Accept-Encoding", "identity")
	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if start > 0 && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		resp.Body.Close()
		// Equal lengths are not proof of equal content: request a fresh representation.
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return t.request(ctx, rawURL, path, false)
	}
	defer resp.Body.Close()
	total, bodySize := resp.ContentLength, resp.ContentLength
	switch resp.StatusCode {
	case http.StatusOK:
		start = 0
	case http.StatusPartialContent:
		if start == 0 {
			return fmt.Errorf("未请求分片却收到 HTTP 206")
		}
		end, size, err := ValidateContentRange(resp.Header.Get("Content-Range"), start)
		if err != nil {
			return err
		}
		bodySize = end - start + 1
		total = size
		if resp.ContentLength >= 0 && resp.ContentLength != bodySize {
			return fmt.Errorf("Content-Range 与 Content-Length 不一致")
		}
	default:
		return StatusError(resp)
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if start > 0 {
		flags = os.O_WRONLY | os.O_APPEND
	}
	out, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return fmt.Errorf("创建/追加文件失败: %w", err)
	}
	reader := &progressReader{body: resp.Body, total: total, current: start, path: path, report: t.Progress}
	n, copyErr := io.Copy(out, reader)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("写入失败: %w", copyErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if bodySize >= 0 && n != bodySize {
		return fmt.Errorf("响应长度不符: expected %d, got %d", bodySize, n)
	}
	if total >= 0 && start+n != total {
		return fmt.Errorf("下载尚未完整: expected %d, got %d", total, start+n)
	}
	return nil
}

func StatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("HTTP 状态码 %d: %s", resp.StatusCode, string(body))
}

func ValidateContentRange(value string, start int64) (int64, int64, error) {
	invalid := fmt.Errorf("无效 Content-Range %q (预期起点 %d)", value, start)
	if !strings.HasPrefix(value, "bytes ") {
		return 0, 0, invalid
	}
	span, totalText, ok := strings.Cut(strings.TrimPrefix(value, "bytes "), "/")
	if !ok {
		return 0, 0, invalid
	}
	firstText, lastText, ok := strings.Cut(span, "-")
	if !ok {
		return 0, 0, invalid
	}
	first, err1 := strconv.ParseInt(firstText, 10, 64)
	last, err2 := strconv.ParseInt(lastText, 10, 64)
	total, err3 := strconv.ParseInt(totalText, 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || first != start || last < first || total <= last {
		return 0, 0, invalid
	}
	return last, total, nil
}

func VerifyFileHash(path, algorithm, expected string) error {
	expected = strings.ToLower(strings.TrimSpace(expected))
	if expected == "" {
		return nil
	}
	file, err := os.Open(path)
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
