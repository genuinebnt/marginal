// docs/api/diagnostics.md §2's REST mapping.
import { GATEWAY_URL } from "./config";
import { apiFetch } from "./http";

export interface Diagnostic {
  analyzer: string;
  severity: "hint" | "warning" | "info";
  message: string;
  block_id?: string;
}

export function getPageDiagnostics(actorId: string, pageId: string): Promise<{ diagnostics: Diagnostic[] }> {
  return apiFetch(`${GATEWAY_URL}/pages/${pageId}/diagnostics`, { actorId });
}

export interface FactDefinition {
  name: string;
  value: string;
  page_id: string;
  block_id: string;
}

export interface FactReference {
  name: string;
  page_id: string;
  block_id: string;
}

export interface FactDuplicateGroup {
  name: string;
  definitions: FactDefinition[];
}

export interface FactsGraph {
  definitions: FactDefinition[];
  duplicates: FactDuplicateGroup[];
  cycle: string[];
  references: FactReference[];
}

export function getFacts(actorId: string): Promise<FactsGraph> {
  return apiFetch(`${GATEWAY_URL}/facts`, { actorId });
}

export function getStaleReferences(actorId: string, factName: string): Promise<FactReference[]> {
  return apiFetch(`${GATEWAY_URL}/facts/${encodeURIComponent(factName)}/stale`, { actorId });
}
