//go:build !js

package appsetup

import (
	"path/filepath"

	"github.com/arnodel/whymark/engine"
)

// openPageStore opens the store of whymark pages in dir: a SQLite
// database.
func openPageStore(dir string) (engine.Store, error) {
	return engine.OpenSQLiteStore(filepath.Join(dir, "whymark.db"))
}
