//go:build js

package browser

import (
	"errors"

	"github.com/arnodel/whymark/internal/whymark"
)

// openPageStore isn't available in a web page, which has no files.
func openPageStore(string) (whymark.Store, error) {
	return nil, errors.New("pages can't be kept in a web page")
}
