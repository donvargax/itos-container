package codec

import manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"

func egressFields(v manifest.Value) bool {
	if v.Kind != manifest.Object || len(v.Fields) != 3 {
		return false
	}
	for _, key := range []string{"internal", "network", "proxy"} {
		r := v.Fields[key]
		if r.Kind != manifest.Object || len(r.Fields) != 3 {
			return false
		}
		for _, name := range []string{"name", "id", "stage"} {
			if r.Fields[name].Kind != manifest.String {
				return false
			}
		}
	}
	return true
}
