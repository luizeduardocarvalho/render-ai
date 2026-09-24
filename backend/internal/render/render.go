// Package render defines the Renderer abstraction used to turn a set of
// input images and a prompt into a photorealistic image, plus the concrete
// Vertex AI implementation and the supporting prompt/pricing/aspect-ratio
// logic used to build a request.
package render

import "context"

// MaxInputImages is the Gemini 3 Pro Image input budget enforced by the
// render assembly step (see API_CONTRACT.md "Image budget").
const MaxInputImages = 14

// RenderRequest is a fully assembled request to an image model: an ordered
// list of input images plus the rendered prompt text.
type RenderRequest struct {
	ModelID     string   // resolved Vertex AI model id to call
	AspectRatio string   // e.g. "16:9"
	ImageSize   string   // "1K", "2K", "4K"
	Images      [][]byte // ordered input images, PNG-encoded
	Prompt      string
}

// RenderResult is what a Renderer call produced. ImageData is nil on a
// failed call (refusal, no image returned, etc.), but the token fields are
// still populated when the model reported usage - Vertex bills input (and
// sometimes thinking) tokens even when no image comes back, so callers can
// still estimate and log that cost. See Renderer.
type RenderResult struct {
	ImageData []byte
	MIMEType  string
	// PromptTokens is the call's billed input tokens.
	PromptTokens int32
	// TextOutputTokens is the call's billed TEXT output tokens only - it
	// deliberately excludes the generated image's own tokens, which are
	// priced per-image instead (see render/pricing.go).
	TextOutputTokens int32
	// ThoughtsTokens is the call's billed thinking-output tokens, if any.
	ThoughtsTokens int32
}

// Renderer generates one image from a RenderRequest. It is an interface so
// other backends (sequential multi-pass, SDXL, ...) can be added later
// without touching the render assembly code.
//
// On error, the returned RenderResult may still carry non-zero token counts
// (see RenderResult) - implementations should populate them whenever the
// underlying API reported usage, even though ImageData is empty.
type Renderer interface {
	Render(ctx context.Context, req RenderRequest) (RenderResult, error)
}
