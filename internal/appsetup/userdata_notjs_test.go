//go:build !js

package appsetup

import (
	"testing"

	"github.com/arnodel/whymark"
)

// TestUserData checks that the latest pages are kept across openings of
// the database, a later page replacing an earlier one.
func TestUserData(t *testing.T) {
	dir := t.TempDir()
	todo := whymark.Document("file:///notes/todo.md")
	user, err := openUserData(dir)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := user.LatestPage(todo); id != "" || err != nil {
		t.Errorf("a new user's latest page is %q, %v", id, err)
	}
	user.SetLatestPage(todo, "first")
	user.SetLatestPage(todo, "second")
	user.SetLatestPage("file:///notes/chat.md", "other")

	again, err := openUserData(dir)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := again.LatestPage(todo); id != "second" || err != nil {
		t.Errorf("after opening the data again, the latest page is %q, %v; want second", id, err)
	}
}
