package provider

import (
	"errors"
	"fmt"
	"testing"

	"github.com/fluencelabs/rauthy-go"
)

func TestScimSyncFailed(t *testing.T) {
	t.Parallel()
	scim := fmt.Errorf("update: %w", &rauthy.APIError{StatusCode: 500, ErrorCode: "Scim"})
	other := &rauthy.APIError{StatusCode: 500, ErrorCode: "Connection"}
	if !scimSyncFailed(scim) {
		t.Error("a wrapped Scim 500 is not recognised")
	}
	if scimSyncFailed(other) || scimSyncFailed(errors.New("boom")) || scimSyncFailed(nil) {
		t.Error("an error other than a Scim 500 is taken for a failed SCIM sync")
	}
}
