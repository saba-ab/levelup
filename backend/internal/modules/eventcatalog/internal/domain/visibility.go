package domain

// ResolveBySlug picks, per slug, the row a tenant actually sees: its own row
// shadows a global row with the same slug. Rows not visible to tenantID are
// ignored, so a caller handing in a broader set cannot leak another tenant's
// definitions.
func ResolveBySlug(tenantID string, rows []EventType) map[string]EventType {
	out := make(map[string]EventType, len(rows))
	for _, r := range rows {
		if !r.VisibleTo(tenantID) {
			continue
		}
		cur, seen := out[r.Slug]
		if !seen || (cur.IsGlobal() && !r.IsGlobal()) {
			out[r.Slug] = r
		}
	}
	return out
}
