# CI fixture only. Provenance, licenses and source references: features/README.md.
# The helper validates the runner's effective numeric UID/GID before building.
FROM docker.io/library/busybox@sha256:5624bb6777cf469b1fb50ae3f981b15d2715ee2fffaf20b9998cfb7843726835
ARG FIXTURE_UID
ARG FIXTURE_GID
USER ${FIXTURE_UID}:${FIXTURE_GID}
ENTRYPOINT []
CMD ["/bin/true"]
