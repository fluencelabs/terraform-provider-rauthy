package provider

import (
	"errors"
	"net/http"
	"testing"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// Rauthy v0.35.2 guards GET /password_policy with session authentication and
// rejects an API key outright. The provider has to recognise that refusal and
// keep prior state, or every refresh of the resource fails.
func TestIsSessionOnly(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		err  error
		want bool
	}{
		"401 from the API":  {newStatusErr(http.StatusUnauthorized), true},
		"403 from the API":  {newStatusErr(http.StatusForbidden), true},
		"404 from the API":  {newStatusErr(http.StatusNotFound), false},
		"500 from the API":  {newStatusErr(http.StatusInternalServerError), false},
		"transport failure": {errors.New("connection refused"), false},
		"no error at all":   {nil, false},
	}

	for name, tc := range cases {
		if got := isSessionOnly(tc.err); got != tc.want {
			t.Errorf("%s: isSessionOnly(%v) = %v, want %v", name, tc.err, got, tc.want)
		}
	}
}

func newStatusErr(status int) error {
	return &client.APIError{
		Method:     http.MethodGet,
		Path:       "/password_policy",
		StatusCode: status,
	}
}
