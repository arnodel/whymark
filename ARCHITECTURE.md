# Architecture

whymark has three parts, and programs that put them together:

- **The protocol** (the root package, `whymark`) is what an engine and a browser say to each
  other: requests, pages, documents.
- **The engine** (`engine`) runs whymark documents: from a request, it makes a page, and it
  keeps pages.
- **The browser** (`browser`) shows documents and pages, and lets the reader interact with
  them. It knows the protocol, not the engine.
- **The programs** (`cmd/…`) build an engine, hand it to a browser, and show the browser with a
  windowing toolkit. `internal/appsetup` holds the choices the two viewers share.

Rendering is [whynot](https://github.com/arnodel/whynot)'s, a library this repository depends on:
whynot lays out and draws Markdown on a `canvas.Canvas`, which Ebitengine and Gio implement.

## Package layout

| Package | What |
|---|---|
| `whymark` | The protocol: `Request`, `Header` (`ReadHeader`, `WriteHeader`), `Follow`, `Document` (`DocumentOf`), `PageInfo`, `IsDocument`, and the `Engine` interface. |
| `engine` | The engine: `Engine`, its stores of pages (`SQLiteStore`, `FileStore`), capabilities and grants. |
| `engine/compile` | Turns a document's source into a `Program`: its passages, its Lua, its settings. Knows nothing of turns. |
| `engine/ai`, `engine/ai/anthropic` | The AI capability, and a provider for it. |
| `browser` | The browser: navigation, history, the journal, selection, the prompt and its microphone, the actions page. |
| `cmd/whymark`, `cmd/giowhymark` | The viewers: the browser in an Ebitengine window, and in a Gio one. |
| `internal/appsetup` | How the viewers are set up: their data directory, their config file, the engine they build (its store, its AI, which documents may use it), and where the user's data is kept. |
| `internal/macapp` | Makes a macOS app bundle around a viewer's binary. |

The dependencies go one way: `browser` and `engine` import `whymark`, never each other; the
programs import all three. `go list -deps ./browser` has no engine package in it.

## The protocol

A browser asks an engine for pages:

- **`Turn(request)`** makes a new page. A request names a document's location, with a passage
  as its fragment and a query; the page it comes from (`FromPageID`), or none to start the
  document; and what was typed (`Input`). Every turn makes a new page, a child of the one it
  comes from: the same request twice gives two siblings.
- The **page** is a stream: a header (its location with its passage, its page-id, its title, the
  document's hints), then its Markdown, as the turn writes it, so it can show while it's made.
- **`Saved(page-id)`** is a page as it was made, with the request it answered: no new page.
- **`Pages(document)`** lists a document's pages, by when they were made, with their parents:
  the tree of what a reader did.
- **`Visit(page-id)`** tells the engine a page was shown, as browsers show pages without asking
  the engine (going back, the journal).

`Follow(header, link)` turns a link written on a page (`#passage`, `?query`) into the next
request. A **document** is identified by its URL without query or fragment (`DocumentOf`), so
the same file at two URLs is two documents. `IsDocument` tells a whymark document by its front
matter. [whymark.md](whymark.md) specifies the format, pages included.

## The engine

A turn goes through three layers: `compile` turns a document into a `Program`, once per content;
`run` runs it, for one turn, in a fresh Lua sandbox; the `Engine` handles requests: it fetches the
document, finds the page the request comes from, runs the program and writes the page.

- **Pages are the engine's state.** Each page records the document's state after its turn, its
  notes, the request it answered, its parent, and its output. With a store (`WithStore`), pages
  outlive the program: `SQLiteStore` keeps them in a database, with when they were made and last
  visited, and can list a document's (`PageLister`) and record visits (`PageVisitor`).
- **Capabilities** are what a document can't do by itself, such as asking an AI. A document
  declares the ones it uses; the program supplies them (`WithCapabilities`) and decides which
  documents get them (`WithGrant`). The AI's answers are recorded with the page.
- A document's own errors are shown in its page; the engine's (a document it can't fetch, a
  page-id it doesn't know) are errors before the page starts.

## The browser

`browser.App` is the browser's state machine, toolkit-independent: the programs feed it input
and draw it on a canvas.

- **Documents and pages.** A plain Markdown document is fetched and shown. A whymark document is
  the engine's: the browser takes a turn and shows the page as it arrives. Turns run in the
  background, so a page waiting for an AI doesn't block the window; its header is applied when
  it arrives. A page still arriving shows waiting dots after a second without growing.
- **The journal** shows the pages that led to the current one, as one scrollable column
  (`ViewStack`). A page opened from its URL loads its trail from the engine in the background.
- **The prompt** is the page's input block; its microphone turns speech into text with a
  `Recognizer` (the Speech framework on macOS, through purego), driven by a `Listener`.
- **What it keeps for the user** goes through `UserData`, which the program provides: in memory
  if it doesn't.
- **Platform services** (the clipboard, opening a web page, system fonts, loading local files,
  speech) are in the browser, one version per platform behind build tags (`_js.go`,
  `_darwin.go`, …), so the programs don't have to provide them.

## The programs

Each viewer is a toolkit's window around a `browser.App`: it builds the engine with
`appsetup.NewEngine`, the user's data with `appsetup.UserData`, and hands both to `NewApp`.

`internal/appsetup` holds this app's choices, which another program would make its own way:

- its data directory, `whymark` in the user's config directory, with `whymark.db`;
- its config file, `whymark/config.yaml` (the AI's key, or a command printing it);
- its engine: the store of pages in `whymark.db`, the AI from Anthropic when there's a key, and
  the AI granted to local documents, or to all with `-remote-ai`;
- the user's data, in tables of their own in `whymark.db` (`user_latest_pages`).

## Where state belongs

State is one of three kinds, and its kind says where it lives:

| Kind | What | Lives | Examples |
|---|---|---|---|
| **The engine's** | What a page *is*: it must come out the same whichever browser shows it. | The engine, and its store. | Documents' state, pages, the AI's answers, dice rolls. |
| **The user's** | What the person chose or did, which should outlive a window. | Kept by the program, on the user's behalf, behind interfaces the browser uses (`UserData`). | The latest page of each document, settings, grants and trust, visits across documents. |
| **The browser's** | What one window is doing right now. | The browser, in memory. | The current page, this window's back and forward, scroll positions, the journal being on, the selection, the prompt's draft. |

To place something new, ask: **does it change what a page contains?** Then it's the engine's.
**Should it outlive the window, as the person's?** Then it's the user's. Otherwise it's the
browser's. Some are close calls, which the question settles: back and forward are the browser's,
a log of visits the user's; theme and zoom are user settings, though today they live in the
browser.

## Where new code goes

- It's about **how documents run**, or what pages are: the engine. If a browser needs to know it,
  it's the protocol's (as whether a document is a whymark document is).
- It's about **what the reader can do**, whatever the toolkit: the browser.
- It's a **choice of this app's** (a path, a provider, a policy): `internal/appsetup`.
- It's about **how one toolkit shows things**: its viewer, in `cmd`.
- A browser needing something from the outside world (the engine, the user's data, a platform
  service) states it as an interface, with a default where one makes sense, and the program
  provides it.

## Planned

- A view of the tree of pages, with `Pages` and `Visit`, drawn on the canvas, in both viewers.
- Pages with owners, for an engine serving several people; share tokens that don't expose a
  page's ancestors (#1).
- Recording which version of a document made each page, so that continuing from a page made by
  an older version can be noticed (#1).
- The protocol over HTTP, for engine-only and browser-only programs, when one is wanted.
