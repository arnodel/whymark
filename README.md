# whymark

whymark is a format for interactive documents, and viewers to read them.

A whymark document is Markdown with a little Lua: its headings are passages, its links move
between them, and its Lua keeps state, remembers where the reader was, and can ask an AI. A
whymark document reads well as plain Markdown, and comes alive in a viewer: a game, a chat, a
notebook, a to-do list you can talk to.

- [whymark.md](whymark.md) is the format's specification.
- [examples](examples) has documents to try: [the cellar](examples/cellar.md),
  [the clockmaker's shop](examples/clockmaker.md), [the notebook](examples/notebook.md),
  [a chat with an AI](examples/chat.md) and [a to-do list](examples/todo.md).

## The viewers

[`whymark`](cmd/whymark) opens Markdown documents and whymark documents, from a file or a URL,
built on [Ebitengine](https://ebitengine.org/); [`giowhymark`](cmd/giowhymark) is the same
viewer built on [Gio](https://gioui.org/). Both render with
[whynot](https://github.com/arnodel/whynot), a Markdown rendering library.

```bash
go install github.com/arnodel/whymark/cmd/whymark@latest
whymark examples/cellar.md
```

See [the viewer's README](cmd/whymark) for what it can do, and the keys.

## How it's organized

- The root package, `whymark`, is the protocol between an engine and a browser: requests,
  pages and their headers, documents.
- [`engine`](engine) is the engine: it compiles whymark documents and runs
  them, turning a request (a document, a passage, input) into a page, and keeps pages.
- [`browser`](browser) is the browser: everything the viewers share, from
  navigation and history to the journal, selection and the prompt.
- [`cmd`](cmd) has the viewers, which wire the two together: each builds an engine, with its
  store of pages and its AI, and hands it to a browser (see
  [`internal/appsetup`](internal/appsetup)). The browser only knows the protocol.

Separating them more clearly, with a protocol between the engine and the browser, is planned
in [#1](https://github.com/arnodel/whymark/issues/1). See
[CONTRIBUTING](CONTRIBUTING.md) for building and testing.

## License

Apache 2.0: see [LICENSE](LICENSE).
