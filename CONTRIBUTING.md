# Contributing to whymark

Thank you for your interest in whymark! This page explains how the project is worked on.
[ARCHITECTURE.md](ARCHITECTURE.md) explains how the code is organized and why.

## Building and testing

whymark needs Go 1.26 or later. It renders with [whynot](https://github.com/arnodel/whynot),
which it requires like any other dependency. To work on both at once, clone whynot next to
whymark and add a `go.work` here (it's ignored by git):

```
go 1.26.0

use (
	.
	../whynot
	../whynot/backends/giobackend
)
```

Run the checks that CI runs:

```bash
gofmt -l .                     # lists any file that needs formatting
go vet ./... && go test ./...
GOOS=js GOARCH=wasm go build -o /dev/null ./cmd/whymark ./cmd/giowhymark
```

On Linux, Gio needs a few system libraries: see
[Gio's installation guide](https://gioui.org/doc/install/linux), or the list in
[`.github/workflows/tests.yaml`](.github/workflows/tests.yaml). On macOS, `make mac-app` and
`make mac-app-gio` build the viewers as app bundles, which the microphone needs, and
`make run-mac-app ARGS=examples/todo.md` runs one from the terminal.

A change that needs something new in whynot needs a whynot release with it before whymark's
`go.mod` can require it: until then, `go.work` builds against the local checkout.

## Proposing a change

- **A feature or a design change starts with an issue** describing it, before any code, so
  that its shape can be discussed first. A bug fix can go straight to a pull request.
- **A pull request** has a short summary of what changes and why, and a test plan: what was
  checked, and how. If it belongs to an issue, it says so: "Part of #N" when the work spans
  several pull requests, or "Closes #N" for the one that completes it.
- **Pull request titles and commit messages** start with a
  [Conventional Commits](https://www.conventionalcommits.org/) type: `feat:`, `fix:`,
  `refactor:`, `docs:`, `test:`, `chore:` or `ci:`, with `!` for a breaking change (`feat!:`).
  Pull requests are squash-merged, so the title becomes the commit message, and the release
  notes group commits by these types.

## Writing comments

Write doc comments for someone *using* the function or type, not for someone reconstructing
how it was built.

- Keep: one or two sentences on what it is or does, the non-obvious details a caller needs to
  use it correctly, and the *why* when there's an obvious alternative someone could reasonably
  prefer.
- Cut: anything already visible in the signature or body; lists of callers; history beyond a
  clause of why the current shape was chosen; design narratives, which belong in the commit
  message or the pull request.
- Don't refer to things a reader of the source can't see, such as conversations or notes
  outside the repository.

Inline comments explain *why* a piece of code is the way it is, not what the next lines do.

## The format

[whymark.md](whymark.md) is the specification of whymark documents. A change to what documents
can say, or to how a viewer treats them, changes it in the same pull request.
