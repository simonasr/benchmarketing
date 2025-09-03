package controller

import (
	"embed"
	"io/fs"
)

// uiFS contains the embedded UI assets.
//
//go:embed ui/*
var uiFS embed.FS

// getUIFS returns a filesystem rooted at the embedded ui directory.
func getUIFS() (fs.FS, error) {
	return fs.Sub(uiFS, "ui")
}
