// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"os"
	"testing"
)

// ciStrictOS returns true when running in CI where all extraction
// tools (tesseract, pdftoppm, pdftotext, magick) are expected to be
// installed. All three platforms now have poppler via native package
// managers (apt on Linux, brew on macOS, MSYS2 pacman on Windows).
func ciStrictOS() bool {
	return os.Getenv("CI") != ""
}

// skipOrFatalCI skips the test when tools/fixtures are missing
// locally, but fails hard in CI where everything should be available.
func skipOrFatalCI(t *testing.T, msg string) {
	t.Helper()
	if ciStrictOS() {
		t.Fatalf("CI: %s", msg)
	}
	t.Skip(msg)
}
