package codec

import manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"

func credentialFields(v manifest.Value) bool {
	if v.Kind != manifest.Object || len(v.Fields) != 6 {
		return false
	}
	for _, key := range []string{"directory", "device", "inode", "stage"} {
		if v.Fields[key].Kind != manifest.String {
			return false
		}
	}
	files := v.Fields["files"]
	if files.Kind != manifest.Array || len(files.Elements) != 2 {
		return false
	}
	for _, id := range files.Elements {
		if id.Kind != manifest.String {
			return false
		}
	}
	k := v.Fields["kinds"]
	if k.Kind != manifest.Object || len(k.Fields) != 2 {
		return false
	}
	for _, key := range []string{"Claude", "GitHub"} {
		if k.Fields[key].Kind != manifest.Boolean {
			return false
		}
	}
	return true
}
