package main

import "fmt"

// knownRoles is ordered; the default (empty WORKER_ROLES) runs all of them —
// that is the demo's single-pod worker (ARCHITECTURE.md §1.1). Production
// sets one role per Deployment.
var knownRoles = []string{"relay", "fanout", "counters", "notifications", "media"}

func resolveRoles(requested []string) ([]string, error) {
	if len(requested) == 0 {
		return knownRoles, nil
	}
	valid := make(map[string]bool, len(knownRoles))
	for _, r := range knownRoles {
		valid[r] = true
	}
	for _, r := range requested {
		if !valid[r] {
			return nil, fmt.Errorf("unknown worker role %q (known: %v)", r, knownRoles)
		}
	}
	return requested, nil
}
