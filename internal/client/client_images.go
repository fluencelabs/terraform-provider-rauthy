package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
)

// ImageKind selects which of a client's two branding images an operation
// addresses. The value doubles as the last path segment.
type ImageKind string

// The two branding images Rauthy stores per client.
const (
	ImageLogo    ImageKind = "logo"
	ImageFavicon ImageKind = "favicon"
)

// Mime types Rauthy accepts for a branding upload. The list is short because
// Rauthy gates on the multipart part's declared type rather than sniffing the
// bytes: anything outside this set is rejected with "Invalid mime type for
// image asset" before the decoder ever runs. Verified against a live 0.36.2 —
// image/webp, image/gif and image/x-icon are all refused on upload even though
// Rauthy itself serves WebP back.
const (
	MimePNG  = "image/png"
	MimeJPEG = "image/jpeg"
	MimeSVG  = "image/svg+xml"
)

// Image is one branding image as Rauthy serves it.
//
// ContentType is what came back, which is not what went up: Rauthy transcodes
// every raster upload to WebP and re-serialises SVG through a sanitiser. See
// PutClientImage.
type Image struct {
	Data        []byte
	ContentType string
}

func clientImagePath(id string, kind ImageKind) string {
	return clientPath(id) + "/" + string(kind)
}

// GetClientImage issues GET /clients/{id}/{logo,favicon}.
//
// A client with no such image and a client that does not exist are both a bare
// 404 with an empty body, so callers cannot tell them apart from this endpoint
// alone. Requires no special right beyond a valid key — the images are served
// to the login page.
func (c *Client) GetClientImage(ctx context.Context, id string, kind ImageKind) (*Image, error) {
	path := clientImagePath(id, kind)
	data, contentType, err := c.download(ctx, path)
	if err != nil {
		return nil, err
	}
	return &Image{Data: data, ContentType: contentType}, nil
}

// PutClientImage uploads a branding image to PUT /clients/{id}/{logo,favicon}.
//
// The OpenAPI document declares no request body at all for this operation. It
// is multipart/form-data: a raw body with an image content type is rejected
// with MultipartError::ContentTypeIncompatible. The part's declared content
// type is load-bearing and trusted — Rauthy dispatches on it (image/svg+xml
// goes to the SVG sanitiser, the others to the raster decoder) rather than
// sniffing, so an honest type must be supplied. The part's *name* is ignored
// entirely; Rauthy takes the first part whatever it is called. We still send
// the documented name.
//
// What is stored is never what was sent. A PNG comes back as WebP, and an SVG
// comes back re-serialised by the sanitiser (attributes reordered, an XML
// declaration prepended), so a 62-byte SVG returns as 101 bytes. The transcode
// is deterministic for a given Rauthy build, which is what makes any hash-based
// drift detection possible at all, but it is not stable across upgrades.
//
// Rasters must be at least 84 pixels on a side; smaller ones are rejected with
// "size must be at least 84 px". SVGs are exempt.
//
// Requires Clients:Update. The document says the logo needs `rauthy_admin`,
// which would put it out of reach of any API key; in practice a key with
// Clients:Update uploads both images fine.
func (c *Client) PutClientImage(ctx context.Context, id string, kind ImageKind, img Image) error {
	path := clientImagePath(id, kind)
	body, contentType, err := multipartBody(string(kind), img)
	if err != nil {
		return fmt.Errorf("build %s %s body: %w", http.MethodPut, path, err)
	}
	return c.upload(ctx, path, contentType, body)
}

// DeleteClientImage issues DELETE /clients/{id}/{logo,favicon}. It is
// idempotent: deleting an image that is not there still answers 200. Requires
// Clients:Update.
func (c *Client) DeleteClientImage(ctx context.Context, id string, kind ImageKind) error {
	return c.do(ctx, http.MethodDelete, clientImagePath(id, kind), nil, nil)
}

// SniffImageMime maps the leading bytes of an image to the content type Rauthy
// expects for it, so that callers need not restate what the file already says.
// Only the three types Rauthy accepts are recognised.
func SniffImageMime(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return MimePNG, nil
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return MimeJPEG, nil
	case isSVG(data):
		return MimeSVG, nil
	default:
		return "", fmt.Errorf("unrecognised image data: Rauthy accepts %s, %s and %s", MimePNG, MimeJPEG, MimeSVG)
	}
}

// isSVG looks for an <svg> root, tolerating a leading XML declaration, a
// doctype, comments or a byte-order mark. SVG has no magic number, so this is
// a heuristic; a caller that knows better can set the type explicitly.
func isSVG(data []byte) bool {
	const window = 1024
	head := data
	if len(head) > window {
		head = head[:window]
	}
	head = bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	return bytes.Contains(bytes.ToLower(head), []byte("<svg")) ||
		strings.HasPrefix(strings.TrimSpace(strings.ToLower(string(head))), "<?xml")
}
