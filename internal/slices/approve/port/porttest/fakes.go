// Package porttest supplies stateful consent fixtures without terminal I/O.
package porttest

import approval "github.com/donvargax/itos-container/internal/engine/approval/port"

// Service retains reviewed state and counts publications.
type Service struct {
	View         approval.View
	Publications int
}

func (s *Service) Inspect() approval.View { return s.View }
func (s *Service) Approve(view approval.View) approval.View {
	s.Publications++
	view.Status = "matching"
	s.View = view
	return view
}

// UI supplies terminal status and a consent line outcome, counting reviews.
type UI struct {
	IsTerminal, Yes bool
	Reviews         int
}

func (u *UI) Terminal() bool            { return u.IsTerminal }
func (u *UI) Review(approval.View) bool { u.Reviews++; return u.Yes }
