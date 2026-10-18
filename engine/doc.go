// Package engine runs whymark documents: Markdown documents written as
// passages with Lua in them (see whymark.md, at the root of the repository,
// for the format). Its Engine is a [whymark.Engine]: it makes pages from
// requests, and keeps them.
//
// The work is in three layers. Package compile turns a document into a
// Program, a value that knows nothing of turns. Running a program, for one
// turn in a fresh sandbox, is this package's run. The Engine handles
// requests: it fetches documents, compiles them (once per content), finds
// the snapshot a request comes from, runs the program, and writes the
// page. None of it knows anything of rendering.
//
// Capabilities, such as the AI (package ai), are what a document can't do
// by itself; the program running the engine provides them, and decides
// which documents get them.
package engine
