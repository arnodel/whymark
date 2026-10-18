// Package appsetup is how the whymark viewers are set up, as programs,
// around the browser they show: their data directory, their config file,
// and the engine they build for it, with its store of pages, its AI, and
// the documents that may use it. These are this app's choices, which
// another program using the browser would make its own way.
package appsetup

import (
	"log"
	"net/url"
	"os"
	"path/filepath"

	"github.com/arnodel/whymark/browser"
	"github.com/arnodel/whymark/engine"
	"github.com/arnodel/whymark/engine/ai"
	"github.com/arnodel/whymark/engine/ai/anthropic"
	"github.com/arnodel/whynot/fetch"
)

// DataDir returns the directory the viewers keep their data in: whymark,
// in the user's config directory.
func DataDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "whymark"), nil
}

// NewEngine returns the engine of a viewer fetching documents through
// registry: its pages kept in the data directory (see DataDir), where
// files can be kept; and, with an API key (see AnthropicAPIKey), the AI
// for documents declaring it, local ones only unless remoteAI is set, as
// the reader pays for what they ask. What goes wrong is logged, and the
// engine goes without.
func NewEngine(registry *fetch.Registry, remoteAI bool) *engine.Engine {
	var options []engine.Option
	if dir, err := DataDir(); err == nil {
		if store, err := openPageStore(dir); err != nil {
			log.Printf("keeping whymark pages: %v", err)
		} else {
			options = append(options, engine.WithStore(store))
		}
	}
	if key, from, err := AnthropicAPIKey(); err != nil {
		log.Printf("the AI's key: %v", err)
	} else if key != "" {
		log.Printf("the AI's key is from %s", from)
		options = append(options,
			engine.WithCapabilities(&ai.AI{Provider: anthropic.New(key), Models: anthropic.Models, Default: "balanced"}),
			engine.WithGrant(func(document *url.URL, name string) bool {
				return remoteAI || isLocal(document)
			}),
		)
	}
	return engine.NewEngine(registry, options...)
}

// UserData returns what the browser keeps for the user (see
// browser.UserData): in the data directory (see DataDir), where files can
// be kept, or else nil, for the browser to keep it in memory. What goes
// wrong is logged.
func UserData() browser.UserData {
	dir, err := DataDir()
	if err != nil {
		return nil
	}
	user, err := openUserData(dir)
	if err != nil {
		log.Printf("keeping the user's data: %v", err)
		return nil
	}
	return user
}

// isLocal reports whether location is a local document's: a file, or one
// built into the browser.
func isLocal(location *url.URL) bool {
	return location.Scheme == "file" || location.Scheme == browser.WelcomeURL.Scheme
}
