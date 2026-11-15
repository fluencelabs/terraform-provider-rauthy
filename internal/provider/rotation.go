package provider

import (
	"errors"
	"net/http"

	"github.com/fluencelabs/rauthy-go"
)

// rotationFailureDetail explains a failed secret rotation, of a client secret
// or an API key. A 3xx or 4xx answer is a refusal; any other failure, a 5xx or
// a connection lost mid-request, may have come after the rotation took effect,
// and the API client does not retry a rotation it has sent, since a retry
// would rotate a second time. aftermath says what that leaves behind for the
// resource at hand.
func rotationFailureDetail(err error, aftermath string) string {
	if apiErr, ok := errors.AsType[*rauthy.APIError](err); ok && apiErr.StatusCode < http.StatusInternalServerError {
		return err.Error()
	}
	return err.Error() + "\n\nRauthy may have rotated the secret even though the request failed, and the " +
		"provider does not retry a rotation, since a retry would rotate it a second time. " + aftermath +
		" The trigger change was not recorded, so the next apply rotates again; if the first rotation " +
		"did take effect, that is a second one."
}
