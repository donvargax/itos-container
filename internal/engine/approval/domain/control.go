package domain

import "strings"

// Exposed compares canonical absolute paths by whole components, including an
// absent prospective location. A worker can write its root and descendants.
func Exposed(location, worker string) bool {
	return location == worker || strings.HasPrefix(location, strings.TrimRight(worker, "/")+"/")
}
