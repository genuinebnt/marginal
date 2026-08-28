package diagnosticsrest

import diagnosticsv1 "marginal/diagnostics-service/genproto/diagnosticsv1"

type diagnosticJSON struct {
	Analyzer string  `json:"analyzer"`
	Severity string  `json:"severity"`
	Message  string  `json:"message"`
	BlockID  *string `json:"block_id,omitempty"`
}

func toAnalyzePageJSON(resp *diagnosticsv1.AnalyzePageResponse) map[string]any {
	diags := resp.GetDiagnostics()
	out := make([]diagnosticJSON, len(diags))
	for i, d := range diags {
		out[i] = diagnosticJSON{Analyzer: d.GetAnalyzer(), Severity: d.GetSeverity(), Message: d.GetMessage(), BlockID: d.BlockId}
	}
	return map[string]any{"diagnostics": out}
}

type definitionJSON struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	PageID  string `json:"page_id"`
	BlockID string `json:"block_id"`
}

func toDefinitionJSON(d *diagnosticsv1.Definition) definitionJSON {
	return definitionJSON{Name: d.GetName(), Value: d.GetValue(), PageID: d.GetPageId(), BlockID: d.GetBlockId()}
}

type duplicateGroupJSON struct {
	Name        string           `json:"name"`
	Definitions []definitionJSON `json:"definitions"`
}

func toReferenceJSON(r *diagnosticsv1.Reference) referenceJSON {
	return referenceJSON{Name: r.GetName(), PageID: r.GetPageId(), BlockID: r.GetBlockId()}
}

type referenceJSON struct {
	Name    string `json:"name"`
	PageID  string `json:"page_id"`
	BlockID string `json:"block_id"`
}

func toAnalyzeFactsJSON(resp *diagnosticsv1.AnalyzeFactsResponse) map[string]any {
	definitions := make([]definitionJSON, len(resp.GetDefinitions()))
	for i, d := range resp.GetDefinitions() {
		definitions[i] = toDefinitionJSON(d)
	}
	duplicates := make([]duplicateGroupJSON, len(resp.GetDuplicates()))
	for i, dup := range resp.GetDuplicates() {
		defs := make([]definitionJSON, len(dup.GetDefinitions()))
		for j, d := range dup.GetDefinitions() {
			defs[j] = toDefinitionJSON(d)
		}
		duplicates[i] = duplicateGroupJSON{Name: dup.GetName(), Definitions: defs}
	}
	cycle := resp.GetCycle()
	if cycle == nil {
		cycle = []string{}
	}
	references := toReferencesJSON(resp.GetReferences())

	return map[string]any{
		"definitions": definitions,
		"duplicates":  duplicates,
		"cycle":       cycle,
		"references":  references,
	}
}

func toReferencesJSON(refs []*diagnosticsv1.Reference) []referenceJSON {
	out := make([]referenceJSON, len(refs))
	for i, r := range refs {
		out[i] = toReferenceJSON(r)
	}
	return out
}
