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

// RenderResult is what a successful Renderer call produced.
type RenderResult struct {
	ImageData    []byte
	MIMEType     string
	PromptTokens int32
	OutputTokens int32
}

// Renderer generates one image from a RenderRequest. It is an interface so
// other backends (sequential multi-pass, SDXL, ...) can be added later
// without touching the render assembly code.
type Renderer interface {
	Render(ctx context.Context, req RenderRequest) (RenderResult, error)
}
