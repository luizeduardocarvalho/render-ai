// Package inventory wraps the Gemini text model calls used for the object
// inventory (view.inventory/generate) and the preservation check's
// removed/added/moved diff.
package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// TextModel calls a Gemini text model for inventory generation and
// preservation checks.
type TextModel struct {
	client  *genai.Client
	modelID string
}

// NewTextModel wraps an existing genai client for text-only calls.
func NewTextModel(client *genai.Client, modelID string) *TextModel {
	return &TextModel{client: client, modelID: modelID}
}

const inventoryPrompt = `You are cataloguing a 3D scene screenshot for an architectural render
pipeline. List every distinct object visible in the image as a concise inventory: one line
per object or object group, including an approximate count and rough position (for example
"2x dining chairs, left of table"). Be terse - this is a checklist, not a description. Do not
comment on camera, lighting, or materials.`

// GenerateInventory asks the text model for a concise object inventory of
// the screenshot (counts and rough positions).
func (t *TextModel) GenerateInventory(ctx context.Context, screenshotPNG []byte) (text string, promptTokens, outputTokens int32, err error) {
	parts := []*genai.Part{
		genai.NewPartFromBytes(screenshotPNG, "image/png"),
		genai.NewPartFromText(inventoryPrompt),
	}
	contents := []*genai.Content{genai.NewContentFromParts(parts, genai.RoleUser)}

	resp, err := t.client.Models.GenerateContent(ctx, t.modelID, contents, &genai.GenerateContentConfig{})
	if err != nil {
		return "", 0, 0, fmt.Errorf("generating inventory: %w", err)
	}

	text = strings.TrimSpace(resp.Text())
	if text == "" {
		return "", 0, 0, fmt.Errorf("text model returned no inventory text")
	}

	if resp.UsageMetadata != nil {
		promptTokens = resp.UsageMetadata.PromptTokenCount
		outputTokens = resp.UsageMetadata.CandidatesTokenCount
	}
	return text, promptTokens, outputTokens, nil
}

// PreservationDiff is the text model's structured judgment of what changed
// between the screenshot and the render, matching API_CONTRACT.md's
// PreservationReport.inventory shape.
type PreservationDiff struct {
	Removed []string `json:"removed"`
	Added   []string `json:"added"`
	Moved   []string `json:"moved"`
	Raw     string   `json:"raw,omitempty"`
}

const preservationPromptTemplate = `Compare IMAGE 1 (the original SketchUp screenshot) to
IMAGE 2 (the photorealistic render produced from it). Using the object inventory below as
reference, identify any objects that were REMOVED, ADDED, or MOVED between the two images.
Ignore purely material, color, or lighting changes - only report object-level differences:
objects missing from the render, extra objects that were not in the original, or objects
that clearly changed position.

Object inventory of IMAGE 1:
%s

Respond with STRICT JSON only - no markdown code fences, no commentary before or after -
matching exactly this shape:
{"removed": ["..."], "added": ["..."], "moved": ["..."], "raw": "one or two sentence summary"}`

// CheckPreservation asks the text model to diff the screenshot against the
// render result, against the cached inventory, and returns the parsed JSON
// verdict. If the model's response is not valid JSON, the raw text is kept
// in Raw and an error is returned so the caller can log it, but the caller
// may still choose to surface the raw text to the user.
func (t *TextModel) CheckPreservation(ctx context.Context, screenshotPNG, resultPNG []byte, inventoryText string) (diff PreservationDiff, promptTokens, outputTokens int32, err error) {
	prompt := fmt.Sprintf(preservationPromptTemplate, inventoryText)

	parts := []*genai.Part{
		genai.NewPartFromBytes(screenshotPNG, "image/png"),
		genai.NewPartFromBytes(resultPNG, "image/png"),
		genai.NewPartFromText(prompt),
	}
	contents := []*genai.Content{genai.NewContentFromParts(parts, genai.RoleUser)}

	resp, err := t.client.Models.GenerateContent(ctx, t.modelID, contents, &genai.GenerateContentConfig{})
	if err != nil {
		return PreservationDiff{}, 0, 0, fmt.Errorf("checking preservation: %w", err)
	}

	if resp.UsageMetadata != nil {
		promptTokens = resp.UsageMetadata.PromptTokenCount
		outputTokens = resp.UsageMetadata.CandidatesTokenCount
	}

	raw := strings.TrimSpace(resp.Text())
	diff, parseErr := parsePreservationJSON(raw)
	if parseErr != nil {
		// Degrade gracefully: surface the raw text so the UI still shows
		// something, but report the parse error to the caller for logging.
		return PreservationDiff{Raw: raw}, promptTokens, outputTokens, fmt.Errorf("parsing preservation JSON: %w", parseErr)
	}
	return diff, promptTokens, outputTokens, nil
}

func parsePreservationJSON(text string) (PreservationDiff, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var diff PreservationDiff
	if err := json.Unmarshal([]byte(text), &diff); err != nil {
		return PreservationDiff{}, err
	}
	if diff.Raw == "" {
		diff.Raw = text
	}
	return diff, nil
}
