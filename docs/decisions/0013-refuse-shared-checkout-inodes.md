# Refuse shared checkout file inodes

Slice-20 extends arc42's **Building-block view** readiness responsibility and
**Runtime view** launch prerequisites. Root and Git directory identity alone
cannot establish the checkout-only mount boundary: a regular file may share its
inode with an unmounted source repository. Read-only permissions do not prevent
the owning UID from changing the mode and overwriting that shared inode.

The shared readiness checkout port now requires exactly one filesystem link for
every regular file recursively below its pinned root, including Git objects,
hidden files, ignored files and untracked files. Directory link counts do not
participate. The disk adapter enumerates pinned directories and observes file
metadata without opening file contents or following symlinks. Unavailable
enumeration or link metadata refuses. Existing root, Git and selected-input
symlink rules remain in force. Go's existing filesystem APIs supply traversal
and native metadata; no new dependency or Git process is needed.

Multiple links refuse conservatively even when both known names are inside the
checkout. The product never copies, unlinks, chmods or chowns to repair the layout.
The safe diagnostic subject is the checkout root, with guidance to prepare an
independent checkout. File-only doctor modes retain their narrower scope.

Admission is fresh for every invocation, after engine inspection, after consent
and at the existing create/start validation boundaries. An independent copy does
not change the approval fingerprint: file bytes and link counts are not settings.
Late refusal still cleans the owned worker using protected authority. Decision
0012 remains binding: mutable checkout contents and new worker-created internal
hard links cannot veto cleanup or recovery.

This relies on the pilot's trusted host and sole checkout mount. It adds no
continuous monitor for trusted host edits during execution. Overlapping workers
and binding the actual mount source to the validated inode remain slice-16 and
slice-17 responsibilities. The local fake/HTTP scenarios establish refusal and
cleanup behavior; HARDLINK-09's native write probe supplies runtime evidence only
in guarded CI, with failed red evidence at the test commit and ordinary green
evidence at the final pushed head.
