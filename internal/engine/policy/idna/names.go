// Package idna adapts golang.org/x/net/idna to the policy's Names port. It maps
// host-name spellings as net/http does, by pure computation: no lookup, no I/O.
package idna

import (
	"strings"

	xidna "golang.org/x/net/idna"
)

// Names canonicalises host names with the library's Lookup profile: lower case,
// UTS 46 non-transitional mapping (full-width forms, ideographic dots and ignored
// code points), STD3 host-name characters, hyphen and Bidi rules and valid A-labels.
type Names struct{}

// ASCII returns the mapped name, or false for any spelling the profile refuses.
// The library's error is dropped on purpose: its text quotes the label.
//
// A final empty A-label is refused too. The library reads xn-- as an empty label
// and returns the text a written trailing dot gives, so no caller could tell the
// two spellings apart, and the one that is no dot would be taken for one.
func (Names) ASCII(name string) (string, bool) {
	ascii, err := xidna.Lookup.ToASCII(name)
	if err != nil {
		return "", false
	}
	if ascii == "" || strings.HasSuffix(ascii, ".") {
		// The name ends in an empty label, written or decoded. Another label after
		// it tells them apart: a dot lets it stand, while xn-- becomes xn--a, which
		// is no A-label. The mapping has already run, so every spelling of xn--
		// (case, full-width forms, ignored code points) is caught alike.
		if _, err := xidna.Lookup.ToASCII(name + "a"); err != nil {
			return "", false
		}
	}
	return ascii, true
}
