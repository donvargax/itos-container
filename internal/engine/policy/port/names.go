package port

// Names canonicalises one host-name spelling. Implementations do no lookup and
// depend on no host state: the same spelling always gives the same answer.
type Names interface {
	// ASCII maps a spelling to its lower-case ASCII form, with Unicode labels
	// as A-labels, and reports false for a spelling its rules refuse. A name
	// may keep one trailing dot, and the answer says nothing about length, label
	// count or whether the name could be an address: the caller checks those.
	// It never returns the refused text or the reason.
	ASCII(name string) (string, bool)
}
