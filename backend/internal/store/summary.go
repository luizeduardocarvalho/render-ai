package store

import "sort"

// summarize projects a full Project down to the lightweight ProjectSummary the
// picker needs, using the first view's screenshot as the thumbnail.
func summarize(p *Project) ProjectSummary {
	s := ProjectSummary{
		ID:        p.ID,
		Name:      p.Name,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
		ViewCount: len(p.Views),
	}
	for _, v := range p.Views {
		s.RenderCount += len(v.Renders)
	}
	if len(p.Views) > 0 && p.Views[0].HasScreenshot {
		s.ThumbnailImageID = p.Views[0].ScreenshotImageID
	}
	return s
}

// sortSummariesNewestFirst orders summaries by UpdatedAt descending (most
// recently touched first), so the picker leads with what the user last worked
// on. Ties fall back to ID for a stable order.
func sortSummariesNewestFirst(s []ProjectSummary) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].UpdatedAt.Equal(s[j].UpdatedAt) {
			return s[i].ID < s[j].ID
		}
		return s[i].UpdatedAt.After(s[j].UpdatedAt)
	})
}

// viewBlobIDs returns every blob ID a view owns: its screenshot, each mask
// bitmap (stored under the mask's own ID), and each render result.
func viewBlobIDs(v *View) []string {
	ids := make([]string, 0, 1+len(v.Masks)+len(v.Renders))
	if v.ScreenshotImageID != "" {
		ids = append(ids, v.ScreenshotImageID)
	}
	for _, m := range v.Masks {
		if m.HasBitmap {
			ids = append(ids, m.ID)
		}
	}
	for _, r := range v.Renders {
		if r.ResultImageID != "" {
			ids = append(ids, r.ResultImageID)
		}
	}
	return ids
}
