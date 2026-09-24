package render

// TokenPricing is USD-per-1M-token input/output rates for one model.
type TokenPricing struct {
	InputPerMTok  float64
	OutputPerMTok float64
}

// ImagePricing is one image model's per-resolution per-image price plus its
// own token rates. The generated image's own tokens are already covered by
// PerImage, so OutputPerMTok only prices TEXT (and thinking) output tokens -
// see ImageCallCost.
type ImagePricing struct {
	PerImage map[string]float64 // resolution -> USD per image
	TokenPricing
}

// PricingTable mirrors config.PricingConfig without importing the config
// package, keeping render independent of it.
type PricingTable struct {
	ProImage   ImagePricing
	FlashImage ImagePricing
	Text       TokenPricing
	UsdToBrl   float64
}

// ImagePricingFor returns the ImagePricing for modelChoice ("pro"/"flash").
// ok is false for any other value.
func (t PricingTable) ImagePricingFor(modelChoice string) (ImagePricing, bool) {
	switch modelChoice {
	case "pro":
		return t.ProImage, true
	case "flash":
		return t.FlashImage, true
	default:
		return ImagePricing{}, false
	}
}

// ImageCallCost returns the estimated USD cost of one image-model call: the
// per-image price for resolution, plus promptTokens (input) and
// textOutputTokens+thoughtsTokens (output) at the model's own rates.
// textOutputTokens must NOT include the generated image's own tokens - those
// are already priced by PerImage, and double-counting them at the text/
// thinking output rate would overstate the cost significantly. ok is false
// if there is no price entry for resolution.
func ImageCallCost(pricing ImagePricing, resolution string, promptTokens, textOutputTokens, thoughtsTokens int32) (cost float64, ok bool) {
	imagePrice, ok := pricing.PerImage[resolution]
	if !ok {
		return 0, false
	}
	outputTokens := textOutputTokens + thoughtsTokens
	tokenCost := float64(promptTokens)/1_000_000*pricing.InputPerMTok +
		float64(outputTokens)/1_000_000*pricing.OutputPerMTok
	return imagePrice + tokenCost, true
}

// TextCallCost returns the estimated USD cost of one text-model call (e.g.
// the preservation check), at the given token rates.
func TextCallCost(pricing TokenPricing, promptTokens, outputTokens int32) float64 {
	return float64(promptTokens)/1_000_000*pricing.InputPerMTok +
		float64(outputTokens)/1_000_000*pricing.OutputPerMTok
}
