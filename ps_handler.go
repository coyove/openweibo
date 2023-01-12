package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/coyove/iis/dal"
	"github.com/coyove/iis/types"
	"github.com/coyove/sdss/contrib/bitmap"
	"github.com/coyove/sdss/contrib/clock"
)

func HandlePublicStatus(w http.ResponseWriter, r *types.Request) {
	stats := dal.TagsStore.DB.Stats()
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(stats)
	w.Write([]byte("\n"))

	fi, err := os.Stat(dal.TagsStore.DB.Path())
	if err != nil {
		fmt.Fprintf(w, "<failed to read data on disk>\n\n")
	} else {
		sz := fi.Size()
		fmt.Fprintf(w, "data on disk: %d (%.2f)\n\n", sz, float64(sz)/1024/1024)
	}

	dal.TagsStore.WalkDesc(clock.UnixMilli(), func(b *bitmap.Range) bool {
		fmt.Fprint(w, b.String())
		return true
	})
}
