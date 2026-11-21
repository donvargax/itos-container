package landlock

// Restrict makes the ruleset and applies it to the calling thread, each raw
// call once: create, set no_new_privs, restrict, close. The ruleset handles
// exactly LANDLOCK_ACCESS_FS_RESOLVE_UNIX with no rule, so the thread cannot
// connect or send to a pathname socket created outside its domain, and
// nothing else changes. There is no weaker mode: a kernel below ABI 9 fails
// the create. Scopes and other file rights are left out on purpose.
//
// A no_new_privs or restrict failure closes the descriptor, and a close error
// during that cleanup does not replace the first failure. A close failure
// after a successful restrict is a failure of its own.
func (a *Adapter) Restrict() error {
	c := a.calls
	fd, err := c.create(rulesetAttr{HandledAccessFS: c.handledFS}, 0)
	if err != nil {
		return classify(err)
	}
	// The prctl call never receives the descriptor.
	if err := c.prctl(c.noNewPrivs, 1, 0, 0, 0); err != nil {
		_ = c.close(fd)
		return err
	}
	if err := c.restrict(fd, 0); err != nil {
		_ = c.close(fd)
		return err
	}
	return c.close(fd)
}
