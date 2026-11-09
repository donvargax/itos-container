package domain

import "strings"

// Exposed compares canonical absolute paths by whole components, including an
// absent prospective location. A worker can write its root and descendants.
func Exposed(location, worker string) bool {
	return location == worker || strings.HasPrefix(location, strings.TrimRight(worker, "/")+"/")
}

// Overlaps reports whether a protected location and a worker checkout are the
// same path or one lies beneath the other. A checkout below a protected tree
// is mounted read/write as itself, so it reaches that tree's records exactly as
// a checkout above it does.
func Overlaps(location, worker string) bool {
	return Exposed(location, worker) || Exposed(worker, location)
}
