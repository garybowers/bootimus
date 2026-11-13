# App releases

A maintainer chooses the version and pushes its Git tag. The release workflow
validates the version, runs the existing CI suite on the tagged commit, then
publishes the binaries and Docker images. It does not publish while a required
check is failing or cancelled.

## Prepare a release

1. Run `make bump NEW_VERSION=0.1.81` (use the agreed version). This updates
   `VERSION`, profile versions and the generated API document. For a candidate,
   use a version such as `0.1.81-rc.1`.
2. Review and merge the version bump. Review the changes since the previous
   release and prepare concise release notes.
3. Tag the merged commit, for example `git tag -a v0.1.81 -m 'Release v0.1.81'`,
   then push only that tag: `git push origin v0.1.81`.
4. Watch the Release workflow. Both publishing jobs wait for the Go build/tests,
   OIDC integration, PostgreSQL, QEMU PXE and browser checks to pass.
5. Check the GitHub assets and checksums, both Docker architectures and the
   release notes. If a check fails, diagnose it before rerunning the failed job.

The tag must match `VERSION` exactly after removing its leading `v`. Versions
use `X.Y.Z` with an optional SemVer prerelease suffix. Build metadata (`+...`)
is rejected because it is not valid in a Docker tag. Tags are limited to
128 characters after removing the `v`.

## Stable releases and prereleases

| Version | GitHub release | Docker tags |
| --- | --- | --- |
| `0.1.81` | Stable, marked latest | `0.1.81`, `latest` |
| `0.1.81-rc.1` | Prerelease, not latest | `0.1.81-rc.1` only |

The same Docker tagging rule applies to `make docker-push` and the local
`make release` target. These local targets publish directly; they do not run
the GitHub CI gate. Use the tag workflow for normal releases.

The workflow publishes Linux and Windows binaries for amd64 and arm64,
`SHA256SUMS`, and Linux Docker images for both architectures. The publishers run
as separate jobs, so check both: a successful GitHub release does not establish
that the Docker push succeeded. Appliance builds and website deployments are
separate.
