package main

import (
	"embed"
	"io/fs"
	"net/http"
)

// Static design mockups, embedded so the binary carries them and nothing has to
// be fetched over HamNet to look at one.
//
// They are deliberately outside the signed-in application: they are a design
// review, not a feature, and they carry no session, no data and no forms that
// post anywhere. The only wiring is two routes and an X-Robots-Tag so a search
// engine does not index four half-finished skins as if they were the site.

//go:embed mockups
var mockupFiles embed.FS

func mockupHandler() (http.Handler, error) {
	sub, err := fs.Sub(mockupFiles, "mockups")
	if err != nil {
		return nil, err
	}
	// The request path still carries /mockups/, so strip it before the file
	// server resolves it against the sub-FS root.
	files := http.StripPrefix("/mockups/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}), nil
}
