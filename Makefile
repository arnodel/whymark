# Local builds of the viewers as macOS apps: "make mac-app" makes
# build/WhyMark.app, from cmd/whymark, and "make mac-app-gio" makes
# build/giowhymark.app, from cmd/giowhymark. Open them with
# "open build/WhyMark.app", or run them from the terminal, with the log
# there, with "make run-mac-app" and "make run-mac-app-gio", passing what
# to open in ARGS:
#
#	make run-mac-app-gio ARGS=examples/todo.md

BUILD := build
ICON := internal/macapp/icon.png
# The latest release tag, without its "v": the bundle shows its numbers.
VERSION := $(patsubst v%,%,$(shell git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null))

.PHONY: mac-app mac-app-gio run-mac-app run-mac-app-gio clean

mac-app:
	go build -o $(BUILD)/bin/whymark ./cmd/whymark
	go run ./internal/macapp -binary $(BUILD)/bin/whymark -name WhyMark -id io.github.arnodel.whymark -version "$(VERSION)" -icon $(ICON) -out $(BUILD)

mac-app-gio:
	go build -o $(BUILD)/bin/giowhymark ./cmd/giowhymark
	go run ./internal/macapp -binary $(BUILD)/bin/giowhymark -name giowhymark -id io.github.arnodel.giowhymark -version "$(VERSION)" -icon $(ICON) -out $(BUILD)

run-mac-app: mac-app
	$(BUILD)/WhyMark.app/Contents/MacOS/whymark $(ARGS)

run-mac-app-gio: mac-app-gio
	$(BUILD)/giowhymark.app/Contents/MacOS/giowhymark $(ARGS)

clean:
	rm -rf $(BUILD)
