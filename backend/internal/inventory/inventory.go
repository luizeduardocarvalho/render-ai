// Package inventory wraps the Gemini text model calls used for the object
// inventory (view.inventory/generate).
package inventory

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// TextModel calls a Gemini text model for inventory generation.
type TextModel struct {
	client  *genai.Client
	modelID string
}

// NewTextModel wraps an existing genai client for text-only calls.
func NewTextModel(client *genai.Client, modelID string) *TextModel {
	return &TextModel{client: client, modelID: modelID}
}

const inventoryPromptEN = `You are cataloguing a 3D scene screenshot for an architectural render
pipeline. List every distinct object visible in the image as a concise inventory: one line
per object or object group, including an approximate count and rough position (for example
"2x dining chairs, left of table"). Be terse - this is a checklist, not a description. Do not
comment on camera, lighting, or materials. Write the inventory in English.`

const inventoryPromptPTBR = `Você está catalogando uma captura de tela de uma cena 3D para um
pipeline de renderização arquitetônica. Liste cada objeto distinto visível na imagem como um
inventário conciso: uma linha por objeto ou grupo de objetos, incluindo uma contagem aproximada
e a posição aproximada (por exemplo, "2x cadeiras de jantar, à esquerda da mesa"). Seja sucinto -
isto é uma checklist, não uma descrição. Não comente sobre câmera, iluminação ou materiais.
Escreva o inventário em português do Brasil.`

// GenerateInventory asks the text model for a concise object inventory of
// the screenshot (counts and rough positions), written in the given
// language ("pt-BR" or anything else, which falls back to English).
// outputTokens includes any thinking tokens the model billed, alongside its
// text output.
func (t *TextModel) GenerateInventory(ctx context.Context, screenshotPNG []byte, language string) (text string, promptTokens, outputTokens int32, err error) {
	prompt := inventoryPromptEN
	if language == "pt-BR" {
		prompt = inventoryPromptPTBR
	}
	parts := []*genai.Part{
		genai.NewPartFromBytes(screenshotPNG, "image/png"),
		genai.NewPartFromText(prompt),
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
		outputTokens = resp.UsageMetadata.CandidatesTokenCount + resp.UsageMetadata.ThoughtsTokenCount
	}
	return text, promptTokens, outputTokens, nil
}
