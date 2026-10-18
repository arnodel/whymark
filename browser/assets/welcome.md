# Welcome to WhyMark {{VERSION}}

{{BUILD}}
**whymark** opens Markdown documents, from a file or a URL, and **whymark documents**:
interactive pages written as Markdown with a little Lua, which keep their state, remember where
you were, and can talk to an AI - games, chats, a to-do list you can speak to. It's built on
[whynot](https://github.com/arnodel/whynot), a Markdown rendering library: only what's on screen
is ever laid out, so even a very large document scrolls and resizes smoothly.

## Open a document

- Start it with `whymark path/to/my/document.md`
- Copy a file path or an `http(s)` URL to your clipboard, then paste it with **{{PASTE_SHORTCUT}}**.
{{ADDRESS_BAR_TIP}}

## Getting around

The three-dot button at the right end of the toolbar, or **{{ACTIONS_SHORTCUT}}**, shows everything
whymark can do, with its keys. Some of it:

- **Mouse wheel** scrolls; resize the window any time to see the document reflow.
- **↓** / **↑** scroll a bit at a time; **Space** / **Shift+Space** page down / up.
- **{{ZOOM_IN_SHORTCUT}}** / **{{ZOOM_OUT_SHORTCUT}}** zoom in / out, and the actions page also
  switches between the light / dark theme.
- Drag the scrollbar, on the right edge, to jump to a position.
- Click a link to follow it: a link to a webpage opens in your browser, while one to a Markdown
  document opens right here - like
  [whynot's README](https://raw.githubusercontent.com/arnodel/whynot/main/README.md).
  **Backspace** (or the back button) goes back, **Shift+Backspace** (or the forward button)
  goes forward.
- The reload button re-fetches the current document.
- **{{JOURNAL_SHORTCUT}}** turns journal mode on or off: the pages you went through to get
  here are shown above this one, as one long document. Follow a link in an earlier page to
  go on from there instead.

Paste "welcome" at any time to come back to this page.

## What it renders

- Headings, paragraphs, emphasis, strong emphasis, and strikethrough
- Inline code, and fenced or indented code blocks
- Ordered, unordered, and nested lists, including task lists
- Tables, with column alignment
- Images - PNG, JPEG, and GIF, including animated GIFs - loaded in the background so they never
  block scrolling
- Blockquotes, including nested ones
- Links, autolinks, and thematic breaks

A quick look at a few of those:

| Left | Center | Right |
|:-----|:------:|------:|
| *italic* | **bold** | ~~struck out~~ |
| `inline code` | a [link](https://github.com/arnodel/whynot) | plain text |

- A list item
  - that can nest
  - as deep as it needs to
- another top-level item

- [x] Renders animated GIFs without blocking the display
- [ ] Renders APNG (not yet)

> A blockquote, for good measure.

---

See [the project on GitHub](https://github.com/arnodel/whymark) for more, including example
whymark documents to try.
