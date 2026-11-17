package provider

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/fluencelabs/rauthy-go"
)

const rauthyHTTPTimeout = 30 * time.Second

func newRauthyClient(url, apiKey string, opts ...rauthy.Option) (*rauthy.Client, error) {
	httpClient := &http.Client{
		Timeout:   rauthyHTTPTimeout,
		Transport: userAttrListTransport{},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return rauthy.New(url, apiKey, append([]rauthy.Option{rauthy.WithHTTPClient(httpClient)}, opts...)...)
}

type userAttrListBodyKey struct{}

type userAttrListTransport struct{}

func (userAttrListTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if body, ok := req.Context().Value(userAttrListBodyKey{}).(*bytes.Buffer); ok && err == nil {
		// Keep the SDK's last attempt and let it read and close the body as usual.
		body.Reset()
		resp.Body = struct {
			io.Reader
			io.Closer
		}{io.TeeReader(resp.Body, body), resp.Body}
	}
	return resp, err
}
