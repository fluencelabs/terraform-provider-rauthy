package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// retryTestPolicy shrinks the budget to something a test can afford. The point
// of the retry code is which requests are re-sent, not how long the pauses are,
// and the pauses are the only expensive part.
func retryTestPolicy() client.Option {
	return client.WithRetryPolicy(4, time.Microsecond, time.Millisecond)
}

// newRetryClient wires a client with the fast policy to a counting server, and
// returns the request counter.
func newRetryClient(t *testing.T, handler http.HandlerFunc) (*client.Client, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(srv.URL, "tf$secret", retryTestPolicy())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, &calls
}

func TestRetry_TransientFailuresThenSuccess(t *testing.T) {
	t.Parallel()

	for _, status := range []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusTooManyRequests,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			var seen atomic.Int32
			c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
				if seen.Add(1) <= 2 {
					w.WriteHeader(status)
					return
				}
				_, _ = io.WriteString(w, `{"id":"u1","email":"a@example.com"}`)
			})

			user, err := c.GetUser(t.Context(), "u1")
			if err != nil {
				t.Fatalf("GetUser after 2 x %d: %v", status, err)
			}
			if user.ID != "u1" {
				t.Errorf("ID = %q, want u1", user.ID)
			}
			if got := calls.Load(); got != 3 {
				t.Errorf("requests = %d, want 3", got)
			}
		})
	}
}

func TestRetry_ExhaustsBudgetAndReportsLastError(t *testing.T) {
	t.Parallel()

	c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":"Internal","message":"upstream is down"}`)
	})

	_, err := c.GetUser(t.Context(), "u1")

	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadGateway || apiErr.Message != "upstream is down" {
		t.Errorf("err = %v, want the last 502 body", apiErr)
	}
	if got := calls.Load(); got != 4 {
		t.Errorf("requests = %d, want the full budget of 4", got)
	}
}

func TestRetry_ClientErrorsAreNotRetried(t *testing.T) {
	t.Parallel()

	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusConflict,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":"BadRequest","message":"nope"}`)
			})

			if _, err := c.GetUser(t.Context(), "u1"); err == nil {
				t.Fatalf("GetUser: want error for HTTP %d", status)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("requests = %d, want 1: HTTP %d will never succeed", got, status)
			}
		})
	}
}

// A Retry-After the client can afford is waited out and the call proceeds; one
// far beyond the honoured ceiling ends the call there and then, which is the
// only externally visible proof that the header was read at all.
func TestRetry_HonoursRetryAfter(t *testing.T) {
	t.Parallel()

	t.Run("short delay is waited out", func(t *testing.T) {
		t.Parallel()

		var seen atomic.Int32
		c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
			if seen.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			_, _ = io.WriteString(w, `{"id":"u1"}`)
		})

		if _, err := c.GetUser(t.Context(), "u1"); err != nil {
			t.Fatalf("GetUser: %v", err)
		}
		if got := calls.Load(); got != 2 {
			t.Errorf("requests = %d, want 2", got)
		}
	})

	t.Run("long delay gives up immediately", func(t *testing.T) {
		t.Parallel()

		c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "600")
			w.WriteHeader(http.StatusServiceUnavailable)
		})

		if _, err := c.GetUser(t.Context(), "u1"); err == nil {
			t.Fatal("GetUser: want error")
		}
		if got := calls.Load(); got != 1 {
			t.Errorf("requests = %d, want 1: a ten-minute wait is not a blip", got)
		}
	})

	t.Run("http-date is understood", func(t *testing.T) {
		t.Parallel()

		c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
			w.WriteHeader(http.StatusServiceUnavailable)
		})

		if _, err := c.GetUser(t.Context(), "u1"); err == nil {
			t.Fatal("GetUser: want error")
		}
		if got := calls.Load(); got != 1 {
			t.Errorf("requests = %d, want 1", got)
		}
	})
}

// A retried write must arrive whole on the second attempt: the body reader is
// consumed by the first one, so it has to be rebuilt rather than reused.
func TestRetry_ReplaysRequestBody(t *testing.T) {
	t.Parallel()

	var seen atomic.Int32
	bodies := make(chan string, 4)
	c, calls := newRetryClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies <- string(raw)
		if seen.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// POST /blacklist rather than POST /users: it is the upsert, and so the one
	// POST this client will re-send after a 503.
	err := c.BlacklistIP(t.Context(), client.IPBlacklistRequest{IP: "192.0.2.7", Exp: 4102444800})
	if err != nil {
		t.Fatalf("BlacklistIP: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}

	close(bodies)
	for body := range bodies {
		var got client.IPBlacklistRequest
		if unmarshalErr := json.Unmarshal([]byte(body), &got); unmarshalErr != nil {
			t.Fatalf("attempt body %q is not the request: %v", body, unmarshalErr)
		}
		if got.IP != "192.0.2.7" || got.Exp != 4102444800 {
			t.Errorf("attempt body = %+v, want the original request", got)
		}
	}
}

// The idempotency rule the whole design turns on: a POST that creates is not
// re-sent once the server has seen it, however transient the failure looks.
func TestRetry_CreatingPOSTIsNotRetried(t *testing.T) {
	t.Parallel()

	c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	_, err := c.CreateUser(t.Context(), client.NewUserRequest{Email: "a@example.com"})
	if err == nil {
		t.Fatal("CreateUser: want error")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("requests = %d, want 1: a 502 may sit on top of a user that was created", got)
	}
}

// A 429 is different: it was refused before the handler ran, so even a create
// may be re-sent.
func TestRetry_CreatingPOSTIsRetriedOnTooManyRequests(t *testing.T) {
	t.Parallel()

	var seen atomic.Int32
	c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if seen.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, `{"id":"u1","email":"a@example.com"}`)
	})

	user, err := c.CreateUser(t.Context(), client.NewUserRequest{Email: "a@example.com"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.ID != "u1" {
		t.Errorf("ID = %q, want u1", user.ID)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
}

// The binary endpoints go through the same plumbing, so they retry too.
func TestRetry_CoversDownload(t *testing.T) {
	t.Parallel()

	var seen atomic.Int32
	c, calls := newRetryClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if seen.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write([]byte{0x00, 0x01})
	})

	img, err := c.GetClientImage(t.Context(), "cl1", client.ImageLogo)
	if err != nil {
		t.Fatalf("GetClientImage: %v", err)
	}
	if img.ContentType != "image/webp" || len(img.Data) != 2 {
		t.Errorf("image = %+v, want the second attempt's body and type", img)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
}

// The upload's pre-encoded multipart body has to be replayed whole, exactly as
// the JSON one does.
func TestRetry_CoversUpload(t *testing.T) {
	t.Parallel()

	var seen atomic.Int32
	sizes := make(chan int, 4)
	c, calls := newRetryClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		sizes <- len(raw)
		if seen.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	img := client.Image{Data: []byte("<svg/>"), ContentType: client.MimeSVG}
	if err := c.PutClientImage(t.Context(), "cl1", client.ImageLogo, img); err != nil {
		t.Fatalf("PutClientImage: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}

	close(sizes)
	var first, second int
	for n := range sizes {
		if first == 0 {
			first = n
			continue
		}
		second = n
	}
	if first == 0 || first != second {
		t.Errorf("body sizes = %d then %d, want the multipart body replayed whole", first, second)
	}
}

// A dead server is a transport failure, and for a GET that is retried until the
// budget runs out.
func TestRetry_ConnectionErrors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	c, err := client.New(url, "tf$secret", retryTestPolicy())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err = c.GetUser(t.Context(), "u1"); err == nil {
		t.Fatal("GetUser: want a connection error")
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		t.Errorf("err = %v, want a transport error rather than an APIError", err)
	}
}

// A cancelled context must end the call rather than sit out the backoff.
func TestRetry_StopsOnCancelledContext(t *testing.T) {
	t.Parallel()

	// A budget with real pauses, so that a retry that ignored the context would
	// visibly outlast the test's own patience.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(srv.URL, "tf$secret", client.WithRetryPolicy(10, 30*time.Second, time.Minute))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		for calls.Load() == 0 { //nolint:revive // spin until the first attempt has landed
		}
		cancel()
	}()

	done := make(chan error, 1)
	go func() { _, getErr := c.GetUser(ctx, "u1"); done <- getErr }()

	select {
	case getErr := <-done:
		if getErr == nil {
			t.Fatal("GetUser: want error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GetUser did not return: the backoff ignored the cancelled context")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}
