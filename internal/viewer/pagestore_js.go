//go:build js

package viewer

import (
	"errors"

	"github.com/arnodel/whymark/engine"
)

// openPageStore isn't available in a web page, which has no files.
func openPageStore(string) (engine.Store, error) {
	return nil, errors.New("pages can't be kept in a web page")
}
