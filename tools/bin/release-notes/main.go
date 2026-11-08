// Command release-notes is a compiling red-first stub for T-25.
// The implementation will be adapted from donvargax/itos-template at
// 79301eba51b9768a27906c720e39c49b08491e67, originally harvested from itos.
package main

const repository = "donvargax/itos-container"

func main() {}

type notes struct {
	Version, Repository, From, Range, Changed string
	Commits                                   []commit
	Upgrading                                 []footer
	Moved                                     []string
}

type commit struct {
	SHA, Header, Type, Breaking string
}

type footer struct {
	SHA, Subject, Text string
}

func breaking(_ string) string                { return "" }
func parseLog(_ string) []commit              { return nil }
func whole(_ []footer, _ string, _ []commit)  {}
func (n *notes) readMoved(_ string) error     { return nil }
func (n notes) render() (string, error)       { return "", nil }
func commits(_ string) ([]commit, error)      { return nil, nil }
func lastRelease(_, _ string) (string, error) { return "", nil }
