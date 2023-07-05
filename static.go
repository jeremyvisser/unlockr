package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"time"
)

var (
	//go:embed index.html
	//go:embed manifest.json
	//go:embed third_party/favicon.png
	//go:embed third_party/favicon-192.png
	//go:embed third_party/favicon-512.png
	//go:embed third_party/favicon.svg
	//go:embed third_party/lit-all.min.js
	//go:embed third_party/lit-all.min.js.map
	staticContentEmbed embed.FS
	staticContent      = FS{staticContentEmbed}
	modTimeEpoch       = time.Now()

	// Revalidation not required while fresh:
	cacheFresh = int((4 * time.Hour).Seconds())
	// If error, serve stale from cache:
	cacheStale = int((40 * time.Hour).Seconds())

	staticHandler = HeaderAdder{
		Handler: http.FileServer(http.FS(staticContent)),
		AddHeaders: http.Header{
			"Cache-Control": []string{fmt.Sprintf("max-age=%d, stale-if-error=%d", cacheFresh, cacheStale)},
		},
	}
)

type HeaderAdder struct {
	http.Handler
	AddHeaders http.Header
}

func (ha HeaderAdder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for h, values := range ha.AddHeaders {
		for _, v := range values {
			w.Header().Add(h, v)
		}
	}
	ha.Handler.ServeHTTP(w, r)
}

// FS wraps embed.FS, but provides sane ModTime values
// that play nicely with http.FileServer.
type FS struct {
	embed.FS
}

func (fs FS) Open(name string) (fs.File, error) {
	f, err := fs.FS.Open(name)
	if err != nil {
		return nil, err
	}
	return File{f}, nil
}

type File struct {
	fs.File
}

func (f File) Stat() (fs.FileInfo, error) {
	fi, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	return FileInfo{fi}, nil
}

type FileInfo struct {
	fs.FileInfo
}

func (fi FileInfo) ModTime() time.Time {
	m := fi.FileInfo.ModTime()
	if m.IsZero() {
		return modTimeEpoch
	}
	return m
}
