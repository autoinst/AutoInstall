package download

import (
	"context"
	"io"
	"net/http"
)

// WithContext binds legacy HTTP requests to the session without discarding each
// request's own timeout. The binding remains alive until the body is closed.
func WithContext(ctx context.Context, client *http.Client) *http.Client {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = http.DefaultClient
	}
	copy := *client
	base := copy.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copy.Transport = contextTransport{ctx: ctx, base: base}
	return &copy
}

type contextTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t contextTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	if t.ctx.Err() != nil {
		cancel()
	}
	resp, err := t.base.RoundTrip(req.Clone(ctx))
	if err != nil {
		stop()
		cancel()
		return nil, err
	}
	resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: func() { stop(); cancel() }}
	return resp, nil
}

type cancelBody struct {
	io.ReadCloser
	cancel func()
}

func (b *cancelBody) Close() error { err := b.ReadCloser.Close(); b.cancel(); return err }
