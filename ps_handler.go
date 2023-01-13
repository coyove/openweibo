package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
)

//go:embed static/assets/*
var httpStaticAssets embed.FS

func HandleIndex(w http.ResponseWriter, r *types.Request) {
	if r.URL.Path != "/" {
		http.Redirect(w, r.Request, "/notfound", 302)
		return
	}
}

func HandleAssets(w http.ResponseWriter, r *types.Request) {
	p := "static/assets/" + strings.TrimPrefix(r.URL.Path, "/assets/")
	buf, _ := httpStaticAssets.ReadFile(p)
	w.Write(buf)
}

func HandlePublicStatus(w http.ResponseWriter, r *types.Request) {
	stats := dal.Store.DB.Stats()
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(stats)
	w.Write([]byte("\n"))

	fi, err := os.Stat(dal.Store.DB.Path())
	if err != nil {
		fmt.Fprintf(w, "<failed to read data on disk>\n\n")
	} else {
		sz := fi.Size()
		fmt.Fprintf(w, "data on disk: %d (%.2f)\n\n", sz, float64(sz)/1024/1024)
	}

	dal.Store.WalkDesc(clock.UnixMilli(), func(b *bitmap.Range) bool {
		fmt.Fprint(w, b.String())
		return true
	})
}
