package searchrest

import documentv1 "marginal/document-service/genproto/documentv1"

type searchHitJSON struct {
	PageID    string  `json:"page_id"`
	PageTitle string  `json:"page_title"`
	BlockID   *string `json:"block_id,omitempty"`
	Snippet   *string `json:"snippet,omitempty"`
	Rank      float32 `json:"rank"`
}

func toSearchResponseJSON(resp *documentv1.SearchResponse) map[string]any {
	hits := resp.GetHits()
	out := make([]searchHitJSON, len(hits))
	for i, h := range hits {
		out[i] = searchHitJSON{
			PageID: h.GetPageId(), PageTitle: h.GetPageTitle(),
			BlockID: h.BlockId, Snippet: h.Snippet, Rank: h.GetRank(),
		}
	}
	return map[string]any{"hits": out}
}

type titleSuggestionJSON struct {
	PageID   string `json:"page_id"`
	Title    string `json:"title"`
	Distance int32  `json:"distance"`
}

func toSuggestResponseJSON(resp *documentv1.SuggestTitlesResponse) map[string]any {
	suggestions := resp.GetSuggestions()
	out := make([]titleSuggestionJSON, len(suggestions))
	for i, s := range suggestions {
		out[i] = titleSuggestionJSON{PageID: s.GetPageId(), Title: s.GetTitle(), Distance: s.GetDistance()}
	}
	return map[string]any{"suggestions": out}
}
