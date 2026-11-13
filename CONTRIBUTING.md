# Contributing to Bootimus

Thanks for your interest in contributing. This guide covers how the project is built, the conventions we follow, and what to check before opening a PR. If something here doesn't fit your change, mention it in the PR description and we'll work it out.

## Repository layout

Bootimus is a monorepo. Several related but independently-built pieces live in this one tree:

- **[cmd/](cmd/) and [internal/](internal/)** — the main Go server binary (`bootimus`). The bulk of the project. Built with `make build`.
- **[web/](web/)** — the admin UI. Vanilla HTML, CSS, and ES modules; embedded into the Go binary at build time via `go:embed`. No build step.
- **[website/](website/)** — the marketing site at bootimus.com. A separate Node/Next project with its own [package.json](website/package.json) and release pipeline (`make build-website` / `make push-website`). Versioned independently.
- **[appliance/](appliance/)** — scripts and overlay used to produce the Bootimus appliance image (a turnkey ISO/disk image). Built with `make appliance`.
- **[distro-profiles.json](distro-profiles.json) and [tools-profiles.json](tools-profiles.json)** — the source-of-truth profile data, embedded into the Go binary via `make sync-profiles`. Versioned independently from the app.
- **[docs/](docs/)** — documentation.

Most PRs touch only one of these. If yours spans more than one (for example, a new feature that needs a server change *and* an admin-UI change), keep them in a single PR but call out the cross-cutting nature in the description.

## Build and run

```
make build      # current platform
make run        # build and run
make clean
```

Cross-compile targets are in the [Makefile](Makefile). The Go toolchain version is pinned in [go.mod](go.mod) — please match it.

## Go code

- **Format with `gofmt`** and make sure files end with a trailing newline.
- **Prefer deletion over preservation.** If you spot a function with no callers while working nearby, removing it is welcome. New code shouldn't add functions without callers.
- **Hold off on abstractions until they pay for themselves.** Three similar lines is usually fine; pull out a helper once there's a third real caller.
- **Validate at system boundaries**, not between two internal functions. Trust your callers and framework guarantees in internal code paths.
- **No comments.** Function and variable names carry the meaning. The only things that look like comments but stay are compiler directives (`//go:build`, `//go:embed`, `//go:generate`, `//nolint`) and `// Code generated ...; DO NOT EDIT.` markers.
- **Wrap errors with `%w`** in sentence form: `fmt.Errorf("listen UDP/67: %w", err)`. Lowercase, no trailing punctuation.
- **Logging** uses the standard `log` package. Lowercase messages, no trailing punctuation.

## Language

- **British English throughout** — documentation, user-facing strings, log messages, commit messages, and identifiers (`normalise`, `colour`, `behaviour`, `initialise`).
- Spellings fixed by an external standard stay as the standard writes them: the HTTP `Authorization` header, `authorized_keys`, CSS properties, and upstream API and library names (`cobra.OnInitialize`, wimlib's `optimize` subcommand).
- Existing URL paths keep their current spelling (`/api/iso-catalog`) — renaming an endpoint is a breaking change, not a spelling fix.

## Tests

We're building test coverage incrementally — there's no expectation that every PR ships with tests, but contributions that add them are appreciated.

- **Pure logic is the easiest place to start.** Functions that take inputs and return outputs (matching, parsing, formatting, validation) test well. See [internal/profiles/manager_test.go](internal/profiles/manager_test.go) for the pattern: table-driven, real input fixtures, no mocks.
- **Avoid mocking the database.** If a test needs storage, it's usually cleaner to refactor the function under test to take a slice or a small local interface rather than the full `storage.Storage`.
- **Stick to the standard library `testing` package.** `t.Errorf` and `t.Fatalf` cover what we need; no extra runners or assertion libraries.
- **Flakes get fixed or deleted.** A test that passes 99/100 times is worse than no test.
- **Run `go test ./...`** before pushing.
- **Integration tests** that need an external service skip themselves unless it's available. `make test-integration` starts a throwaway OIDC provider ([mock-oauth2-server](https://github.com/navikt/mock-oauth2-server)) in Docker and runs the OIDC sign-in tests against it; CI runs this on every pull request.

If your PR changes behaviour in a tested package, ideally add a test that fails on the old behaviour and passes on the new. If that's hard without an invasive refactor, just call it out in the PR description.

## PostgreSQL tests

The storage, server and admin test suites run on SQLite by default. Set `BOOTIMUS_TEST_POSTGRES_DSN` to a PostgreSQL URL and the same tests run against PostgreSQL instead: every test gets its own schema (created and dropped around it), so a shared database is fine. `make test-postgres` does this end to end with a throwaway `postgres` container, picking the image tag that matches your local `pg_dump` so the dump format and the server agree. CI runs the same packages against PostgreSQL 16 on every pull request.

Two things to know when adding storage code:

- **Use the shared store helpers.** Inside `internal/storage` call `newTestStore(t)`; from other packages call `storagetest.New(t)`. Both return a migrated store on whichever backend the environment selects and clean up after the test. Tests that reach into SQLite specifics (for example a deliberately unmigrated database file) stay on `storage.NewSQLiteStore`.
- **SQLite does not enforce foreign keys, PostgreSQL does.** A delete that works on SQLite can fail on PostgreSQL when rows still reference the deleted one, so detach or cascade explicitly. `TestPostgresBackupRoundTrip` also exercises `pg_dump` and `psql` through the backup export and staged restore; it skips without the DSN or the client tools.

## End-to-end boot test

When updating iPXE, test BIOS and UEFI, network and USB boot, and USB keyboard input. Check menu navigation, cancellation and keyboard input after chaining into another bootloader, such as GRUB. Include affected physical hardware: passing QEMU/OVMF tests does not establish compatibility with vendor firmware.

`make test-e2e` boots real virtual machines against a real bootimus. It builds the binary, starts it, and runs these stages, each of which writes its serial console and the server log under `.e2e/run/`:

| Stage | What it asserts |
|-------|-----------------|
| BIOS menu | SeaBIOS's iPXE ROM fetches `undionly.kpxe`, the embedded script chains `autoexec.ipxe`, the client is auto-discovered with hardware inventory, the menu is requested and its title appears on the serial console |
| UEFI menu | The same through OVMF with `bootimus.efi`. If the firmware never reaches iPXE, the loader is launched from a FAT volume instead and the test says so |
| Alpine next boot | An Alpine ISO is downloaded through the admin API with SHA-256 verification, extracted, given Alpine's netboot initramfs as an initrd override, set as the client's next boot, and booted. The kernel and initrd are served over HTTP, the modloop and apk repository come from the extracted ISO tree on the boot port, an apkovl overlay fetches the install callback URL from the auto-install endpoint, reports completion and powers the VM off, and the install record ends up completed |
| proxyDHCP events | Bridge mode only: every booted MAC has a replied entry in the proxyDHCP request log |

There are two network modes, chosen automatically:

- **Bridge** (needs root or passwordless `sudo`, plus `iproute2`): the suite creates a bridge, a tap device for the VMs and a veth pair into a private network namespace where a tiny DHCP server (`e2e/dhcpd`, built from the repo) hands out addresses with no PXE options. Bootimus runs with `--proxy-dhcp --bind-interface <bridge>` on UDP/67 and UDP/69, so the real proxyDHCP, TFTP and HTTP paths are exercised exactly as on a LAN. The binary therefore needs `cap_net_bind_service`. Interface names carry the test's pid, leftovers from earlier runs are removed first, and everything is torn down on cleanup even when a stage fails.
- **User-mode** (`slirp`, no privileges): QEMU's own TFTP serves the loaders and a copy of `autoexec.ipxe`, and the guest reaches bootimus's HTTP ports at 10.0.2.2. Bootimus's TFTP and proxyDHCP are not exercised and the suite says so. Do not try to point the slirp guest at a TFTP server on the host: libslirp intercepts UDP/69 addressed to 10.0.2.2 and answers from its built-in server, so packets never reach the host and the client sees "Permission denied" when no `tftp=` directory is set.

`BOOTIMUS_E2E_NETWORK=bridge|slirp` forces a mode and `BOOTIMUS_E2E_REQUIRE_BRIDGE=1` (set in CI) fails fast if bridge mode cannot be set up. Locally, bridge mode without `sudo` can run inside a user namespace, where the test is root for its own network stack:

```bash
make e2e-binary
unshare -rnm sh -c 'mount -t tmpfs tmpfs /run && mkdir -p /run/netns && ip link set lo up && BOOTIMUS_E2E_BINARY=$PWD/.e2e/bootimus BOOTIMUS_E2E_NETWORK=bridge make test-e2e'
```

You need `qemu-system-x86_64`, an OVMF firmware (the `ovmf` or `edk2-ovmf` package) and `bsdtar` (`libarchive-tools`), which bootimus uses to extract the ISO with full file names. KVM is used when `/dev/kvm` is accessible; without it the VMs run under TCG with longer timeouts. The Alpine files are cached under `~/.cache/bootimus-e2e` (override with `BOOTIMUS_E2E_CACHE`) and only downloaded once. Ports default to 8080 for boot HTTP and 18081 for the admin API; `BOOTIMUS_E2E_HTTP_PORT` and `BOOTIMUS_E2E_ADMIN_PORT` move them if they clash. CI uploads `.e2e/run/` as an artefact when the job fails.

## UI smoke tests

`make test-ui` drives the admin UI in a real browser with [Playwright](https://playwright.dev). It builds the binary, starts it with a temporary SQLite data directory on ports 16969 (TFTP), 18080 (boot HTTP) and 18081 (admin), sets a known admin password through `bootimus user set-password`, creates a read-only user over the API, runs the suite in headless Chromium and always stops the server afterwards.

The tests live in `ui-tests/tests/` and stay at smoke level: signing in and out, every sidebar tab rendering without a page error or console error, creating a client and opening its detail page and menu preview, creating a deployment profile, an enrolment rule with its live match count and a SAN target, API tokens and two-factor set-up, housekeeping, backup export, the PXE Debug banner, the audit log and the read-only view. Each test is independent and uses random names and MAC addresses, so the suite can be re-run against the same server.

To run it locally you need Node 22 or newer:

```bash
cd ui-tests && npm ci && npx playwright install chromium && cd ..
make test-ui
```

`make test-ui -- tests/clients.spec.ts` style filtering is not supported; run `ui-tests/run.sh tests/clients.spec.ts` instead with `BOOTIMUS_UI_BINARY` pointing at a built binary, or `npx playwright test` inside `ui-tests/` against a server you started yourself with `BOOTIMUS_UI_URL`, `BOOTIMUS_UI_ADMIN_PASSWORD` and `BOOTIMUS_UI_VIEWER_PASSWORD` exported. The ports move with `BOOTIMUS_UI_ADMIN_PORT`, `BOOTIMUS_UI_HTTP_PORT` and `BOOTIMUS_UI_TFTP_PORT`, and `BOOTIMUS_UI_KEEP_DATA=1` keeps the data directory and server log after a run.

On failure Playwright keeps a screenshot, a trace and a page snapshot under `ui-tests/test-results/`; open a trace with `npx playwright show-trace <file>.zip`. CI runs the suite on every pull request and uploads `ui-tests/playwright-report/` and `ui-tests/test-results/` when it fails.

To add a test, create a `*.spec.ts` next to the others and import `test`, `expect` and the helpers from `./helpers`: `loginAsAdmin`, `loginAsViewer`, `openTab`, `openClientsSubtab`, `createClientViaUI`, `randomMAC` and `uniqueSuffix`. The `test` export auto-accepts confirm dialogs and fails any test whose page throws an uncaught exception. Prefer role, label and id selectors; add a `data-testid` only where nothing stable exists. Keep each test under a few seconds and independent of the others.

## Cross-platform code

Bootimus builds for Linux (primary), macOS, and Windows. When a syscall, path, or API differs by OS:

- Use Go build tags rather than runtime `runtime.GOOS` checks: `//go:build !windows` and `//go:build windows` in separate files named `foo_unix.go` / `foo_windows.go`.
- On Windows, prefer `golang.org/x/sys/windows` over the frozen `syscall` package. One exception: `syscall.Handle` for socket FD conversion has no `x/sys/windows` equivalent.
- If a function has no callers on a given platform, deleting it is preferable to porting it.

## Frontend (admin UI)

The admin UI in [web/static/](web/static/) is intentionally vanilla — plain HTML, CSS, and ES modules with no build step, framework, or bundler. Please keep it that way:

- No npm dependencies, build steps, or frameworks in the admin UI.
- Match the existing CSS variable system and component patterns rather than reskinning sections in isolation.
- Action buttons live in toolbars, not in card titles or section headers.
- View-mode toggles use an icon plus tooltip rather than a one-word label like "Tree".

The marketing site under [website/](website/) is a separate codebase with its own `package.json`. We use **yarn** there, not npm.

## Configuration

There's one example config, [bootimus.example.yaml](bootimus.example.yaml). Rather than adding OS- or scenario-specific copies, please extend the existing example with a comment explaining when to enable any new keys.

## Distro profiles

[distro-profiles.json](distro-profiles.json) and [tools-profiles.json](tools-profiles.json) are the source of truth for boot behaviour. Please bump the `version` field when you change them so caches invalidate cleanly.

## App releases

Releases are selected by a maintainer and published by the existing
[release workflow](.github/workflows/release.yml). Pushing a `v*` tag starts
the workflow, which publishes Linux and Windows binaries, checksums and Docker
images. Stable releases also update Docker's `latest` tag.

Release checklist:

1. Run `make bump NEW_VERSION=X.Y.Z`. This updates `VERSION`, the source and
   embedded profile versions, and `docs/openapi.json` using the Go toolchain.
2. Review and merge those changes, then wait for CI on the commit to pass.
3. Tag that commit and push only the intended tag, for example
   `git push origin vX.Y.Z`.
4. Check both publishing jobs, the release assets and Docker images, then
   review the generated release notes.

The local `make release` target also pushes Docker images and prints manual
GitHub upload instructions. Use the tag workflow for the usual release process.
Website deployments and appliance builds are separate.

## Before opening a PR

- [ ] `gofmt -l .` is clean
- [ ] `go build ./...` passes
- [ ] `go vet ./...` is clean
- [ ] `go test ./...` passes
- [ ] If you touched anything platform-specific: `GOOS=windows go build ./...` and `GOOS=darwin go build ./...` both pass
- [ ] Files end with a trailing newline
- [ ] No new dependencies unless the PR description explains why
- [ ] PR title is short and describes the change

## A few things we'd rather avoid

- **Drive-by formatting or refactoring** in code unrelated to your PR. It makes review harder.
- **Marketing-style copy** in READMEs or release notes. Plain descriptions of what changed read better.
- **Decorative emoji** in code, comments, commit messages, or docs.
