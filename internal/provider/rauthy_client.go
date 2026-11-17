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

type userAttrListResponseKey struct{}

type userAttrListResponseCapture struct {
	body   bytes.Buffer
	status int
}

type userAttrListTransport struct{}

func (userAttrListTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	capture, ok := req.Context().Value(userAttrListResponseKey{}).(*userAttrListResponseCapture)
	if !ok {
		return resp, err
	}
	// Keep the SDK's last attempt and let it read and close the body as usual.
	capture.body.Reset()
	capture.status = 0
	if err != nil {
		return resp, err
	}
	capture.status = resp.StatusCode
	resp.Body = struct {
		io.Reader
		io.Closer
	}{io.TeeReader(resp.Body, &capture.body), resp.Body}
	return resp, nil
}
