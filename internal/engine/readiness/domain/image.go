package domain

import (
	"slices"
	"strconv"
	"strings"

	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func compatibleAPI(v port.Version) bool {
	minimum, a := apiNumber(v.Minimum)
	maximum, b := apiNumber(v.Maximum)
	used, c := apiNumber(v.Negotiated)
	sdkMin, d := apiNumber(v.SDKMinimum)
	sdkMax, e := apiNumber(v.SDKMaximum)
	return a && b && c && d && e && minimum <= maximum && sdkMin <= sdkMax && used >= minimum && used <= maximum && used >= sdkMin && used <= sdkMax
}

func apiNumber(value string) (int, bool) {
	major, minor, ok := strings.Cut(value, ".")
	if !ok || major != "1" || !decimal(minor) {
		return 0, false
	}
	n, err := strconv.Atoi(minor)
	return n, err == nil
}

func validImageID(id string) bool {
	digest, ok := strings.CutPrefix(id, "sha256:")
	return ok && len(digest) == 64 && strings.Trim(digest, "0123456789abcdef") == ""
}

func imageIssues(image port.Image, request manifest.Settings) []port.Issue {
	var issues []port.Issue
	if !slices.Contains(image.RepoDigests, request.Image) {
		issues = append(issues, port.Issue{Kind: port.ImageDigest, Subject: request.Image})
	}
	if image.OS != "linux" || image.Architecture != "amd64" {
		issues = append(issues, port.Issue{Kind: port.ImagePlatform, Subject: request.Image})
	}
	if !matchesUser(image.User, request.ContainerUser, request.RemoteUser) {
		issues = append(issues, port.Issue{Kind: port.ImageUser, Subject: request.Image})
	}
	return issues
}

func matchesUser(user string, requested ...*string) bool {
	uid, gid, group := strings.Cut(user, ":")
	if !decimal(uid) || uid[0] == '0' || (group && !decimal(gid)) {
		return false
	}
	for _, value := range requested {
		if value != nil && *value != uid {
			return false
		}
	}
	return true
}

func decimal(value string) bool { return value != "" && strings.Trim(value, "0123456789") == "" }
