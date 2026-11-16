package presentation

import (
	"fmt"
	"slices"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// CredentialLines projects validated frozen grants for approval or status. Only
// display is escaped; canonical bytes and structured JSON retain literal paths.
func CredentialLines(grants []port.CredentialGrant, network bool) []string {
	if len(grants) == 0 {
		return nil
	}
	lines := grantLines(grants)
	lines = append(lines, port.CredentialExposure, port.CredentialStaging, port.CredentialOutput)
	if network {
		lines = append(lines, port.CredentialNetwork, port.CredentialLogs)
	}
	return lines
}

func grantLines(grants []port.CredentialGrant) []string {
	lines := make([]string, 0, len(grants))
	for _, g := range grants {
		lines = append(lines, fmt.Sprintf("credentials: %s source=%s target=%s", g.Kind, jsonText(g.Source), jsonText(g.Target)))
	}
	return lines
}

func grants(effective any) []port.CredentialGrant {
	s, _ := effective.(map[string]any)
	values, _ := s["credentials"].([]any)
	var result []port.CredentialGrant
	for _, raw := range values {
		g, _ := raw.(map[string]any)
		kind, _ := g["kind"].(string)
		source, _ := g["source"].(string)
		target, _ := g["target"].(string)
		result = append(result, port.CredentialGrant{Kind: kind, Source: source, Target: target})
	}
	return result
}

func requested(v port.View) []port.CredentialGrant {
	if v.Inspection.Ready || v.Inspection.Policy == nil || v.Inspection.Policy.Policy == nil {
		return nil
	}
	var result []port.CredentialGrant
	for _, g := range v.Inspection.Policy.Policy.Credentials {
		result = append(result, port.CredentialGrant{Kind: g.Kind, Source: g.Source, Target: "/run/itos-container/credentials/" + g.Kind})
	}
	slices.SortFunc(result, func(a, b port.CredentialGrant) int { return strings.Compare(a.Kind, b.Kind) })
	return result
}

func restriction(effective any) string {
	s, _ := effective.(map[string]any)
	profile, _ := s["profile"].(string)
	mount := "checkout-only host mount at /workspace"
	denied := "no ports, grants or hooks"
	if len(grants(effective)) > 0 {
		mount = "checkout-only host directory mount at /workspace plus read-only credential files under /run/itos-container/credentials"
		denied = "no ports or hooks"
	}
	network := "network none"
	if allowlist(effective) != nil {
		network = "internal network with no route out"
	}
	return profile + ": " + mount + "; UID:GID override; no supplementary groups; read-only root; dropped capabilities; no-new-privileges; " + network + "; " + denied + "; ephemeral tmpfs; pids limit 4096"
}

func (u UI) credentialDisclosure(effective any) {
	for _, line := range CredentialLines(grants(effective), allowlist(effective) != nil) {
		_, _ = fmt.Fprintln(u.Out, line)
	}
	s, _ := effective.(map[string]any)
	p, _ := s["policy"].(map[string]any)
	if p["version"] == float64(3) {
		_, _ = fmt.Fprintln(u.Out, port.CredentialGate)
	}
}
