package api

import (
	"net/http"

	renderpkg "render-ai/backend/internal/render"
)

// assumedPromptTokens and assumedOutputTokens model a typical render request
// for the GET /api/pricing preview - enough context images (screenshot,
// region map, edge map, a couple of asset refs) plus a modest prompt/
// response, before any real render has run to measure one from. They
// deliberately exclude preservation-check tokens: the preview prices one
// model+resolution combination, not a whole request.
const (
	assumedPromptTokens int32 = 6000
	assumedOutputTokens int32 = 500 // thinking/text output combined
)

// pricingEstimate is one model+resolution combination's estimated cost.
type pricingEstimate struct {
	Model      string  `json:"model"`
	Resolution string  `json:"resolution"`
	CostUsd    float64 `json:"costUsd"`
	CostBrl    float64 `json:"costBrl"`
}

// pricingResponse is the body of GET /api/pricing.
type pricingResponse struct {
	UsdToBrl  float64           `json:"usdToBrl"`
	Estimates []pricingEstimate `json:"estimates"`
}

// pricingResolutions lists the resolutions offered for each model in the UI
// (see RenderControls / API_CONTRACT.md's resolution guard).
var pricingResolutions = map[string][]string{
	"pro":   {"1K", "2K", "4K"},
	"flash": {"1K"},
}

// getPricing estimates the USD/BRL cost of one render for every
// model+resolution combination the UI offers, using an assumed typical
// request (assumedPromptTokens/assumedOutputTokens) since no real usage is
// available before a render actually runs. It requires only a signed-in
// user (see Router) - it's a price list, not project data.
func (s *Server) getPricing(w http.ResponseWriter, r *http.Request) error {
	table := s.pricingTable()

	var estimates []pricingEstimate
	for _, model := range []string{"pro", "flash"} {
		pricing, ok := table.ImagePricingFor(model)
		if !ok {
			continue
		}
		for _, resolution := range pricingResolutions[model] {
			cost, ok := renderpkg.ImageCallCost(pricing, resolution, assumedPromptTokens, assumedOutputTokens, 0)
			if !ok {
				continue
			}
			estimates = append(estimates, pricingEstimate{
				Model:      model,
				Resolution: resolution,
				CostUsd:    cost,
				CostBrl:    cost * table.UsdToBrl,
			})
		}
	}

	writeJSON(w, http.StatusOK, pricingResponse{
		UsdToBrl:  table.UsdToBrl,
		Estimates: estimates,
	})
	return nil
}
