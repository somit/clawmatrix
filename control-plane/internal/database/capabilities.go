package database

// CapabilityGroups maps group names to the list of capabilities they grant.
// When an agent's role changes, reassign groups rather than editing individual capabilities.
var CapabilityGroups = map[string][]string{
	"support_readonly": {
		"client.search",
		"orders.status",
		"portfolio.summary",
	},
	"mf_execution": {
		"mf.sip.create_draft",
		"mf.sip.modify",
		"mf.sip.submit",
	},
}

// ExpandGroups resolves a list of group names into the full set of capabilities.
// Unknown group names are included as-is (treated as individual capabilities).
func ExpandGroups(groups []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range groups {
		if caps, ok := CapabilityGroups[g]; ok {
			for _, c := range caps {
				if !seen[c] {
					seen[c] = true
					out = append(out, c)
				}
			}
		} else {
			if !seen[g] {
				seen[g] = true
				out = append(out, g)
			}
		}
	}
	return out
}
