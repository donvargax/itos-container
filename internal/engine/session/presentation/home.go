package presentation

import (
	"encoding/json"
	"fmt"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (u UI) PrintHomes(r port.HomeResult, withJSON bool) int {
	problems := []Problem{}
	code := 0
	for _, f := range r.Failures {
		p := problem(f)
		problems = append(problems, p)
		code = max(code, p.Exit)
	}
	if r.Homes == nil {
		r.Homes = []port.HomeEntry{}
	}
	if withJSON {
		_ = json.NewEncoder(u.Out).Encode(struct {
			Schema   int              `json:"schema"`
			OK       bool             `json:"ok"`
			Problems []Problem        `json:"problems"`
			Homes    []port.HomeEntry `json:"homes"`
		}{1, code == 0, problems, r.Homes})
	} else {
		for _, e := range r.Homes {
			data, _ := json.Marshal(e)
			_, _ = fmt.Fprintln(u.Out, string(data))
		}
		if len(r.Homes) == 0 && code == 0 {
			_, _ = fmt.Fprintln(u.Out, "No retained homes.")
		}
	}
	for _, p := range problems {
		_, _ = fmt.Fprintf(u.Err, "%s: %s %s\n", p.Rule, p.Message, p.Fix)
	}
	return code
}
