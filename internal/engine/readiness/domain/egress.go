package domain

import (
	"strconv"
	"strings"

	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// The version is a prerequisite, never a substitute for post-create verification.
func egressVersion(v port.Version) bool {
	parts := strings.Split(v.Release, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if !decimal(part) {
			return false
		}
	}
	major, err := strconv.ParseUint(parts[0], 10, 32)
	api, ok := apiNumber(v.Negotiated)
	return err == nil && major >= 28 && ok && api >= 48
}

func (i Inspector) inspectProxy(r *port.Result, session port.Session) {
	image, failure := session.Image(i.ProxyImage)
	if failure != port.None {
		if failure == port.ImageMissing {
			failure = port.EgressImageMissing
		} else {
			failure = port.EgressImage
		}
	} else if !image.ConfigPresent || !validImageID(image.ID) || image.Volumes || len(i.imageIssues(image, manifest.Settings{Image: i.ProxyImage})) != 0 {
		failure = port.EgressImage
	}
	if failure != port.None {
		r.Issues = append(r.Issues, port.Issue{Kind: failure, Subject: i.ProxyImage})
		return
	}
	if i.Executables == nil {
		failure = port.EgressExecutable
	} else {
		observed, f := i.Executables.Observe()
		failure = ProxyExecutable(observed, f, r.Checkout.Root)
	}
	if failure != port.None {
		r.Issues = append(r.Issues, port.Issue{Kind: failure})
		return
	}
	r.ProxyImage = &port.ImageIdentity{Reference: i.ProxyImage, ID: image.ID, OS: image.OS, Architecture: image.Architecture}
}
