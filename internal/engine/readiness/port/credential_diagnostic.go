package port

import "strconv"

func CredentialDiagnostic(issue CredentialIssue) Problem {
	p := Problem{Rule: issue.Rule, Exit: 1, File: "itos-container.yaml"}
	if issue.Index >= 0 {
		path := "/credentials/" + strconv.Itoa(issue.Index) + "/source"
		p.Path = &path
	}
	switch issue.Rule {
	case "credential.missing":
		p.Message, p.Fix, p.Exit = "Credential source is absent", "provide the requested source at a safe host path", 3
	case "credential.unavailable":
		p.Message, p.Fix, p.Exit = "Credential metadata or exclusion evidence is unavailable", "provide complete stable source, checkout and mount metadata", 3
	case "credential.mount_id":
		p.Message, p.Fix, p.Exit = "Credential mount identity STATX_MNT_ID is unavailable", "use a host kernel that exposes STATX_MNT_ID for every source and protected boundary", 3
	case "credential.filesystem":
		p.Message, p.Fix, p.Exit = "Credential filesystem is outside the accepted ext4, btrfs and xfs set. Overlay or FUSE backing paths cannot be excluded from credential exposure, nor can paths on NFS or 9p", "use source, checkout and state paths on ext4, btrfs or xfs", 3
	case "credential.symlink":
		p.Message, p.Fix = "Credential path contains a symbolic link", "use a source with no symbolic link in any path component"
	case "credential.ancestor":
		p.Message, p.Fix = "Credential ancestor is unsafe", "use root- or host-owned ancestors without group or other write permissions"
	case "credential.location":
		p.Message, p.Fix = "Credential source is exposed in a protected tree", "use an independent source outside checkout and protected state"
	case "credential.regular":
		p.Message, p.Fix = "Credential source is not a regular file", "provide a private regular file"
	case "credential.owner":
		p.Message, p.Fix = "Credential source belongs to another user", "use a file owned by the effective host user"
	case "credential.permissions":
		p.Message, p.Fix = "Credential source permissions expose it to another user", "remove every group and other permission bit"
	case "credential.links":
		p.Message, p.Fix = "Credential source has another filesystem link", "provide an independent file with exactly one link"
	case "credential.size":
		p.Message, p.Fix = "Credential source size is outside the supported bounds", "provide a file of 1 through 65536 bytes"
	case "credential.duplicate":
		p.Message, p.Fix = "Credential source repeats an earlier inode", "use independent files for each credential kind"
	case "state.location", "state.path", "state.unsafe", "state.unavailable":
		return StateDiagnostic(issue.Rule, "")
	case "checkout.unavailable":
		return Diagnostic(Issue{Kind: CheckoutUnavailable, Subject: issue.Subject})
	case "checkout.path":
		return Diagnostic(Issue{Kind: CheckoutPath, Subject: issue.Subject})
	case "checkout.root":
		return Diagnostic(Issue{Kind: CheckoutRoot, Subject: issue.Subject})
	case "checkout.changed":
		return Diagnostic(Issue{Kind: CheckoutChanged, Subject: issue.Subject})
	case "host.unsupported":
		return Diagnostic(Issue{Kind: HostUnsupported})
	default:
		p.Message, p.Fix, p.Exit = "Unknown credential inspection failure", "report this as a bug", 70
	}
	return p
}
