package domain

import "strings"

// NestedCheckouts compares canonical Linux roots by whole path components.
// Equal roots preserve the existing same-checkout concurrency contract.
func NestedCheckouts(a, b string) bool {
	if a == b {
		return false
	}
	return strings.HasPrefix(a, strings.TrimRight(b, "/")+"/") || strings.HasPrefix(b, strings.TrimRight(a, "/")+"/")
}
