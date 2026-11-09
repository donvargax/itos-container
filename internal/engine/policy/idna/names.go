// Package idna adapts golang.org/x/net/idna to the policy's Names port. It maps
// host-name spellings as net/http does, by pure computation: no lookup, no I/O.
package idna

import xidna "golang.org/x/net/idna"

// Names canonicalises host names with the library's Lookup profile: lower case,
// UTS 46 non-transitional mapping (full-width forms, ideographic dots and ignored
// code points), STD3 host-name characters, hyphen and Bidi rules and valid A-labels.
type Names struct{}

// ASCII returns the mapped name, or false for any spelling the profile refuses.
// The library's error is dropped on purpose: its text quotes the label.
func (Names) ASCII(name string) (string, bool) {
	ascii, err := xidna.Lookup.ToASCII(name)
	if err != nil {
		return "", false
	}
	return ascii, true
}
