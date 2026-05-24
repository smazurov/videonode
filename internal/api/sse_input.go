package api

import "github.com/danielgtaylor/huma/v2"

// sseInput is a Huma operation input whose Resolve hook sets
// Cache-Control: no-cache on the response. Without it, Firefox routes
// text/event-stream responses through its HTTP cache layer, which applies
// a read timeout to sparse streams and tears the connection down after
// ~25-30s. MDN lists this header as required for SSE; Huma's sse helper
// only sets Content-Type, so we wire it in via a resolver per the pattern
// documented in danielgtaylor/huma#392.
type sseInput struct{}

// Resolve sets Cache-Control: no-cache on the SSE response.
func (*sseInput) Resolve(ctx huma.Context) []error {
	ctx.SetHeader("Cache-Control", "no-cache")
	return nil
}
