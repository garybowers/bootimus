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

## Tests

We're building test coverage incrementally — there's no expectation that every PR ships with tests, but contributions that add them are appreciated.

- **Pure logic is the easiest place to start.** Functions that take inputs and return outputs (matching, parsing, formatting, validation) test well. See [internal/profiles/manager_test.go](internal/profiles/manager_test.go) for the pattern: table-driven, real input fixtures, no mocks.
- **Avoid mocking the database.** If a test needs storage, it's usually cleaner to refactor the function under test to take a slice or a small local interface rather than the full `storage.Storage`.
- **Stick to the standard library `testing` package.** `t.Errorf` and `t.Fatalf` cover what we need; no extra runners or assertion libraries.
- **Flakes get fixed or deleted.** A test that passes 99/100 times is worse than no test.
- **Run `go test ./...`** before pushing.

If your PR changes behaviour in a tested package, ideally add a test that fails on the old behaviour and passes on the new. If that's hard without an invasive refactor, just call it out in the PR description.

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

- **GitHub Actions and release automation.** Releases are cut manually — please don't add workflow files.
- **Drive-by formatting or refactoring** in code unrelated to your PR. It makes review harder.
- **Marketing-style copy** in READMEs or release notes. Plain descriptions of what changed read better.
- **Decorative emoji** in code, comments, commit messages, or docs.
