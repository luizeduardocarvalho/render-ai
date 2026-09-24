package render

import "testing"

func testPricingTable() PricingTable {
	return PricingTable{
		ProImage: ImagePricing{
			PerImage: map[string]float64{
				"1K": 0.134,
				"2K": 0.134,
				"4K": 0.24,
			},
			TokenPricing: TokenPricing{InputPerMTok: 2.00, OutputPerMTok: 12.00},
		},
		FlashImage: ImagePricing{
			PerImage:     map[string]float64{"1K": 0.039},
			TokenPricing: TokenPricing{InputPerMTok: 0.50, OutputPerMTok: 3.00},
		},
		Text:     TokenPricing{InputPerMTok: 0.30, OutputPerMTok: 2.50},
		UsdToBrl: 5.16,
	}
}

func TestImageCallCost(t *testing.T) {
	pricing := testPricingTable().ProImage

	// Image tokens (the generated image's own tokens, already covered by the
	// per-image price) must never be added in - only prompt, text output and
	// thinking tokens are billed on top of the per-image price.
	cost, ok := ImageCallCost(pricing, "2K", 5000, 200, 100)
	if !ok {
		t.Fatalf("expected a price entry for 2K")
	}
	wantTokenCost := 5000.0/1_000_000*2.00 + (200.0+100.0)/1_000_000*12.00
	want := 0.134 + wantTokenCost
	if diff := cost - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("cost = %v, want %v", cost, want)
	}

	// Zero tokens still charges exactly the per-image price.
	cost, ok = ImageCallCost(pricing, "4K", 0, 0, 0)
	if !ok || cost != 0.24 {
		t.Errorf("zero-token cost = %v, %v, want 0.24, true", cost, ok)
	}
}

func TestImageCallCostMissingResolution(t *testing.T) {
	pricing := testPricingTable().FlashImage // only has "1K"

	if _, ok := ImageCallCost(pricing, "2K", 1000, 100, 0); ok {
		t.Error("expected ok=false for a resolution with no price entry")
	}

	// An empty ImagePricing (e.g. an unknown model choice) also reports no
	// price rather than silently costing 0.
	if _, ok := ImageCallCost(ImagePricing{}, "1K", 1000, 100, 0); ok {
		t.Error("expected ok=false for an empty pricing table")
	}
}

func TestPricingTableImagePricingFor(t *testing.T) {
	table := testPricingTable()

	if _, ok := table.ImagePricingFor("pro"); !ok {
		t.Error("expected pro to resolve")
	}
	if _, ok := table.ImagePricingFor("flash"); !ok {
		t.Error("expected flash to resolve")
	}
	if _, ok := table.ImagePricingFor("nope"); ok {
		t.Error("expected an unknown model choice to report ok=false")
	}
}

func TestTextCallCost(t *testing.T) {
	pricing := testPricingTable().Text

	cost := TextCallCost(pricing, 3000, 300)
	want := 3000.0/1_000_000*0.30 + 300.0/1_000_000*2.50
	if diff := cost - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("cost = %v, want %v", cost, want)
	}

	if got := TextCallCost(pricing, 0, 0); got != 0 {
		t.Errorf("zero-token text cost = %v, want 0", got)
	}
}
