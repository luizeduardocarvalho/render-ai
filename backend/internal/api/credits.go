// Credit pricing helpers - the one place unit<->credit conversion and the
// per-variation render price happen, so the rest of the package (charging in
// render.go, the admin grant endpoint, /api/me/credits) never duplicates the
// arithmetic. See API_CONTRACT.md's credits section and PRICING.md.
package api

import (
	"fmt"
	"math"

	"render-ai/backend/internal/store"
)

// unitsPerCredit is the store's internal integer resolution: balances,
// charges and grants are all stored as this many "units" per whole credit.
// 4 means a quarter-credit (the flash price, and the smallest grant
// increment) is always a whole number of units, never a float rounding
// problem.
const unitsPerCredit = 4

// maxGrantCredits bounds a single admin credit grant/correction, in credits
// (see API_CONTRACT.md's admin section: |amount| <= 10000).
const maxGrantCredits = 10000

// unitsPerVariation is the render.go charge for one variation of the given
// model+resolution combination, in integer units - see PRICING.md:
//   - pro 1K/2K: 1 credit    (4 units)
//   - pro 4K:    2 credits   (8 units)
//   - flash:     0.25 credit (1 unit)
//
// model/resolution are assumed already validated by validateRenderRequest;
// an unrecognized resolution falls back to the pro 1K/2K price rather than
// charging nothing.
func unitsPerVariation(model store.ModelChoice, resolution store.Resolution) int64 {
	if model == store.ModelFlash {
		return 1
	}
	if resolution == store.Resolution4K {
		return 2 * unitsPerCredit
	}
	return unitsPerCredit
}

// unitsToCredits converts the store's integer units to the credits number the
// JSON API speaks.
func unitsToCredits(units int64) float64 {
	return float64(units) / float64(unitsPerCredit)
}

// creditsToUnits converts a credits amount received from outside (the admin
// grant endpoint's request body) into integer units, rejecting anything that
// isn't a multiple of 1/unitsPerCredit (0.25 credits).
func creditsToUnits(credits float64) (int64, error) {
	scaled := credits * unitsPerCredit
	rounded := math.Round(scaled)
	if math.Abs(scaled-rounded) > 1e-6 {
		return 0, fmt.Errorf("must be a multiple of %.2f credits", 1.0/float64(unitsPerCredit))
	}
	return int64(rounded), nil
}
