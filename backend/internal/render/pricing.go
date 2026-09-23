package render

// PricingTable mirrors config.PricingConfig without importing the config
// package, keeping render independent of it.
type PricingTable struct {
	ImagePro      map[string]float64 // resolution -> USD per image
	ImageFlash    map[string]float64
	InputPerMTok  float64
	OutputPerMTok float64
}

// EstimateCost returns the estimated USD cost of a render call: the
// per-image price for modelChoice ("pro"/"flash") and resolution, plus the
// text-token cost of the same call. ok is false if there is no price entry
// for the given model/resolution combination.
func EstimateCost(pricing PricingTable, modelChoice, resolution string, promptTokens, outputTokens int32) (cost float64, ok bool) {
	var table map[string]float64
	switch modelChoice {
	case "pro":
		table = pricing.ImagePro
	case "flash":
		table = pricing.ImageFlash
	default:
		return 0, false
	}
	imagePrice, ok := table[resolution]
	if !ok {
		return 0, false
	}
	tokenCost := float64(promptTokens)/1_000_000*pricing.InputPerMTok +
		float64(outputTokens)/1_000_000*pricing.OutputPerMTok
	return imagePrice + tokenCost, true
}
