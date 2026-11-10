package port

func (r Record) egressTransition(n Record) bool {
	if r.Schema != 3 && n.Schema != 3 {
		return r.Egress == (EgressRecord{}) && n.Egress == (EgressRecord{})
	}
	if !n.ValidEgressShape() {
		return false
	}
	if r.Schema == 2 {
		return r.Egress == (EgressRecord{}) && r.Stage == "reserved" && n.Stage == r.Stage && n.ContainerID == "" && r.Anchor == n.Anchor && r.Anchor.Stage == "owned" &&
			n.Egress.Internal.Stage == "intent" && n.Egress.Network.Stage == "" && n.Egress.Proxy.Stage == ""
	}
	if r.Schema != 3 || n.Schema != 3 || !r.ValidEgressShape() {
		return false
	}
	before := []EgressResource{r.Egress.Internal, r.Egress.Network, r.Egress.Proxy}
	after := []EgressResource{n.Egress.Internal, n.Egress.Network, n.Egress.Proxy}
	changes := 0
	for i, a := range before {
		b := after[i]
		if a == b {
			continue
		}
		changes++
		if a.Name != b.Name || r.Stage != n.Stage || r.ContainerID != n.ContainerID || r.Anchor != n.Anchor {
			return false
		}
		switch a.Stage {
		case "":
			if b.Stage != "intent" || r.Stage != "reserved" {
				return false
			}
			for _, v := range before[:i] {
				if v.Stage != "owned" {
					return false
				}
			}
		case "intent":
			if b.Stage != "owned" || !HexID(b.ID) {
				return false
			}
		case "owned":
			if b.Stage != "absent" || b.ID != a.ID || (r.Stage != "reserved" && r.Stage != "cancelled" && r.Stage != "absent") {
				return false
			}
			for _, v := range before[i+1:] {
				if v.Stage != "" && v.Stage != "absent" {
					return false
				}
			}
		default:
			return false
		}
	}
	return changes <= 1
}
