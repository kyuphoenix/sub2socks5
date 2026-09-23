package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"sub2socks5-go/internal/app"
)

//go:embed internal/public/*
var embeddedPublic embed.FS

//go:embed version
var embeddedVersion string

func main() {
	staticFS, err := fs.Sub(embeddedPublic, "internal/public")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := app.RunWithStaticFSAndVersion(staticFS, embeddedVersion); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
