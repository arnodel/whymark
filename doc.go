// Package whymark is the protocol of whymark documents: what an engine,
// which runs documents, and a browser, which shows them, say to each
// other. whymark.md, at the root of the repository, specifies the format.
//
// A browser asks an [Engine] for pages. A [Request] names a document, at
// a passage, from the page it comes from, with what was typed; the
// Engine's answer is a page, a stream starting with a [Header] (see
// [ReadHeader] and [WriteHeader]) followed by the page's Markdown, which
// shows as it's made. [Follow] turns a link on a page into the next
// request. Pages are kept: [Engine.Saved] shows one again,
// [Engine.Pages] lists a [Document]'s, and [Engine.Visit] tells the engine
// a page was shown.
//
// Package engine runs documents, in the same program as a browser or
// elsewhere.
package whymark
