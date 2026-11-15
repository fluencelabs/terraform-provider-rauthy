// Package rauthy is a thin HTTP client for the Rauthy admin API (/auth/v1),
// together with the value constraints Rauthy enforces on OIDC clients.
//
// The request and response types, and the contract tests that check them
// against Rauthy's own OpenAPI document, track Rauthy v0.36.2. The client
// authenticates with a Rauthy API key and retries transient failures (see
// WithRetryPolicy).
//
// A minimal use:
//
//	c, err := rauthy.New("https://auth.example.com", apiKey)
//	if err != nil {
//		return err
//	}
//	created, err := c.CreateClient(ctx, rauthy.NewClientRequest{
//		ID:           "example-app",
//		Confidential: true,
//		RedirectURIs: []string{"https://app.example.com/callback"},
//	})
//	if err != nil {
//		return err
//	}
//	fmt.Println(created.ID)
package rauthy
