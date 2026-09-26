// Package inventory wraps the Gemini text model calls used for the object
// and material inventory (view.inventory/generate).
package inventory

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
pipeline. The catalogue is used twice: as a checklist that every object must remain in the
render, and as the material specification the renderer follows, because the screenshot only
shows flat colors.

List every distinct object visible in the image, and every major surface (floor, walls,
ceiling, countertops), one line per object, object group or surface, in this format:

<count>x <object>, <rough position> - <material>

The material part says what the thing is most plausibly made of, in physical terms the
renderer can reproduce: the specific material (for example "European oak", "Carrara marble",
"powder-coated steel", "boucle fabric"), its finish (matte, satin, gloss, brushed, honed) and,
when it applies, its texture scale and direction (for example "planks about 18 cm wide running
left to right", "60x60 cm tiles with thin grout"). Base it on the color, texture, shape and
context you see, and keep the color you see: a light grey sofa stays light grey. When several
materials are plausible, pick the most likely one for this kind of room. Never leave the
material out and never answer just "wood" or "metal".

Do not invent objects that are not visible. Do not comment on camera or lighting. Be terse -
this is a checklist, not a description. Example:

Floor, whole room - wide-plank European oak, matte oil finish, planks about 18 cm wide running left to right
2x dining chairs, left of table - walnut frame, light grey boucle fabric seat
1x pendant lamp, above table - brushed brass shade, satin finish

Write the catalogue in English.`

const inventoryPromptPTBR = `Você está catalogando uma captura de tela de uma cena 3D para um
pipeline de renderização arquitetônica. O catálogo é usado duas vezes: como checklist de que
todos os objetos devem permanecer na renderização, e como especificação de materiais que o
renderizador segue, porque a captura de tela mostra apenas cores chapadas.

Liste cada objeto distinto visível na imagem, e cada superfície principal (piso, paredes, teto,
bancadas), uma linha por objeto, grupo de objetos ou superfície, neste formato:

<quantidade>x <objeto>, <posição aproximada> - <material>

A parte do material diz do que a coisa é mais provavelmente feita, em termos físicos que o
renderizador consiga reproduzir: o material específico (por exemplo "carvalho europeu",
"mármore Carrara", "aço com pintura eletrostática", "tecido bouclé"), seu acabamento (fosco,
acetinado, brilhante, escovado, polido) e, quando se aplica, a escala e a direção da textura
(por exemplo "réguas de cerca de 18 cm de largura no sentido esquerda-direita", "porcelanato
60x60 cm com rejunte fino"). Baseie-se na cor, na textura, na forma e no contexto que você vê, e
mantenha a cor que você vê: um sofá cinza claro continua cinza claro. Quando vários materiais
forem plausíveis, escolha o mais provável para esse tipo de ambiente. Nunca deixe o material de
fora e nunca responda apenas "madeira" ou "metal".

Não invente objetos que não estejam visíveis. Não comente sobre câmera ou iluminação. Seja
sucinto - isto é uma checklist, não uma descrição. Exemplo:

Piso, ambiente todo - carvalho europeu em réguas largas, acabamento fosco com óleo, réguas de cerca de 18 cm no sentido esquerda-direita
2x cadeiras de jantar, à esquerda da mesa - estrutura em nogueira, assento em tecido bouclé cinza claro
1x pendente, acima da mesa - cúpula em latão escovado, acabamento acetinado

Escreva o catálogo em português do Brasil.`

// The user's material notes are appended to the prompt when there are any. The
// render prompt lets the notes win over the list, so a list that repeats the
// floor as "oak" while the notes say "dark wood" would give the renderer two
// specs for one surface. The catalogue therefore has to fold the notes in.
const materialNotesIntroEN = `

The user also wrote these material notes for the whole scene:

%s

Treat the notes as instructions, not as extra objects. Where a note names an object or
surface, use that material on its line instead of your own guess, and fill in the detail the
note leaves out (finish, texture scale, direction) so the line stays specific. The list must
never contradict the notes. Objects the notes do not mention keep your own guess.`

const materialNotesIntroPTBR = `

O usuário também escreveu estas notas de materiais para a cena inteira:

%s

Trate as notas como instruções, não como objetos extras. Quando uma nota citar um objeto ou
superfície, use esse material na linha dele no lugar do seu palpite, e complete o detalhe que a
nota deixa de fora (acabamento, escala da textura, direção) para a linha continuar específica.
A lista nunca deve contradizer as notas. Objetos que as notas não citam ficam com o seu palpite.`

// buildPrompt returns the catalogue prompt in the given language ("pt-BR" or
// anything else, which falls back to English), with the material notes folded
// in when the user wrote any.
func buildPrompt(language, materialNotes string) string {
	prompt, intro := inventoryPromptEN, materialNotesIntroEN
	if language == "pt-BR" {
		prompt, intro = inventoryPromptPTBR, materialNotesIntroPTBR
	}
	notes := strings.TrimSpace(materialNotes)
	if notes == "" {
		return prompt
	}
	// The closing "Write the catalogue in ..." line stays last, so the notes
	// go before it.
	i := strings.LastIndex(prompt, "\n\n")
	return prompt[:i] + fmt.Sprintf(intro, notes) + prompt[i:]
}

// Vertex answers 429 RESOURCE_EXHAUSTED when the per-minute quota of the model
// is used up, which usually clears within seconds. The inventory call is cheap
// and a user is waiting on it, so it retries a few times with backoff before
// giving up. Retries are per call (not on the shared client) so renders keep
// their own failure handling.
const retryAttempts int32 = 3

// Delays are in seconds. Variables so tests can run the retry loop instantly.
var (
	retryInitialDelay = 2.0
	retryMaxDelay     = 8.0
	retryJitter       = 1.0
)

func retryOptions() *genai.HTTPRetryOptions {
	attempts, initial, maxDelay, jitter := retryAttempts, retryInitialDelay, retryMaxDelay, retryJitter
	return &genai.HTTPRetryOptions{
		Attempts:        &attempts,
		InitialDelay:    &initial,
		MaxDelay:        &maxDelay,
		Jitter:          &jitter,
		HTTPStatusCodes: []int32{http.StatusTooManyRequests, http.StatusServiceUnavailable},
	}
}

// IsRateLimited reports whether err is Vertex telling us its quota is
// exhausted (HTTP 429 / RESOURCE_EXHAUSTED), as opposed to a real failure.
func IsRateLimited(err error) bool {
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == http.StatusTooManyRequests || apiErr.Status == "RESOURCE_EXHAUSTED"
	}
	return false
}

// GenerateInventory asks the text model for a concise object inventory of
// the screenshot (counts, rough positions and materials), written in the given
// language ("pt-BR" or anything else, which falls back to English). Any
// materialNotes the user wrote for the scene are folded into the materials.
// outputTokens includes any thinking tokens the model billed, alongside its
// text output.
func (t *TextModel) GenerateInventory(ctx context.Context, screenshotPNG []byte, language, materialNotes string) (text string, promptTokens, outputTokens int32, err error) {
	prompt := buildPrompt(language, materialNotes)
	parts := []*genai.Part{
		genai.NewPartFromBytes(screenshotPNG, "image/png"),
		genai.NewPartFromText(prompt),
	}
	contents := []*genai.Content{genai.NewContentFromParts(parts, genai.RoleUser)}

	resp, err := t.client.Models.GenerateContent(ctx, t.modelID, contents, &genai.GenerateContentConfig{
		HTTPOptions: &genai.HTTPOptions{RetryOptions: retryOptions()},
	})
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
