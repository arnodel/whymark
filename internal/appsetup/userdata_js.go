//go:build js

package appsetup

import (
	"errors"

	"github.com/arnodel/whymark/browser"
)

// openUserData isn't available in a web page, which has no files.
func openUserData(string) (browser.UserData, error) {
	return nil, errors.New("user data can't be kept in a web page")
}
