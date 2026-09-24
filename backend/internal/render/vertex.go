package render

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// VertexRenderer calls the Gemini image models on Vertex AI, one request per
// render (no sequential/multi-pass modes - those are left to future
// Renderer implementations).
type VertexRenderer struct {
	client *genai.Client
}

// NewVertexRenderer builds a Vertex AI client using Application Default
// Credentials. The image models require the "global" location.
func NewVertexRenderer(ctx context.Context, project, location string) (*VertexRenderer, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:  genai.BackendVertexAI,
		Project:  project,
		Location: location,
	})
	if err != nil {
		return nil, fmt.Errorf("creating vertex ai client: %w", err)
	}
	return NewVertexRendererFromClient(client), nil
}

// NewVertexRendererFromClient wraps an already-constructed genai client, so
// callers that also need a text model (see internal/inventory) can share one
// client/credential set instead of creating two.
func NewVertexRendererFromClient(client *genai.Client) *VertexRenderer {
	return &VertexRenderer{client: client}
}

// Render implements Renderer.
func (r *VertexRenderer) Render(ctx context.Context, req RenderRequest) (RenderResult, error) {
	parts := make([]*genai.Part, 0, len(req.Images)+1)
	for _, img := range req.Images {
		parts = append(parts, genai.NewPartFromBytes(img, "image/png"))
	}
	parts = append(parts, genai.NewPartFromText(req.Prompt))
	contents := []*genai.Content{genai.NewContentFromParts(parts, genai.RoleUser)}

	resp, err := r.client.Models.GenerateContent(ctx, req.ModelID, contents, &genai.GenerateContentConfig{
		ResponseModalities: []string{"IMAGE", "TEXT"},
		ImageConfig: &genai.ImageConfig{
			AspectRatio: req.AspectRatio,
			ImageSize:   req.ImageSize,
		},
	})
	if err != nil {
		return RenderResult{}, fmt.Errorf("vertex ai generate content: %w", err)
	}

	return extractResult(resp)
}

// extractResult reads the image (if any) and usage metadata out of resp. It
// always populates the token fields of its returned RenderResult, even when
// it also returns an error for "no image" - a refusal or safety block still
// bills input (and sometimes thinking) tokens, so the caller can still
// estimate and log that cost instead of losing it silently.
func extractResult(resp *genai.GenerateContentResponse) (RenderResult, error) {
	var result RenderResult
	var notes strings.Builder

	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, p := range cand.Content.Parts {
			if p.InlineData != nil && len(p.InlineData.Data) > 0 {
				result.ImageData = p.InlineData.Data
				result.MIMEType = p.InlineData.MIMEType
			}
			if p.Text != "" {
				if notes.Len() > 0 {
					notes.WriteString(" ")
				}
				notes.WriteString(p.Text)
			}
		}
	}

	if resp.UsageMetadata != nil {
		um := resp.UsageMetadata
		result.PromptTokens = um.PromptTokenCount
		result.ThoughtsTokens = um.ThoughtsTokenCount
		// CandidatesTokenCount lumps the output image's own tokens together
		// with any text/thinking output, which would double-bill the image
		// (already priced per-image) at the text output rate. Split by
		// modality and keep only TEXT - fall back to the lumped total when
		// the API doesn't return the breakdown, so cost accounting degrades
		// gracefully instead of silently going to zero.
		if len(um.CandidatesTokensDetails) > 0 {
			for _, d := range um.CandidatesTokensDetails {
				if d.Modality == genai.MediaModalityText {
					result.TextOutputTokens += d.TokenCount
				}
			}
		} else {
			result.TextOutputTokens = um.CandidatesTokenCount
		}
	}

	if result.ImageData == nil {
		msg := "model returned no image"
		switch {
		case notes.Len() > 0:
			msg = fmt.Sprintf("model returned no image (refused): %s", notes.String())
		case len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != "":
			msg = fmt.Sprintf("model returned no image (finish reason: %s)", resp.Candidates[0].FinishReason)
		}
		return result, errors.New(msg)
	}

	return result, nil
}
