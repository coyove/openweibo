package middleware

import (
	"bytes"
	"encoding/binary"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

var Survey struct {
	Max     int64
	Written int64
}

var engine *gin.Engine

func loadTrafficCounter() {
	tmppath := filepath.Join(os.TempDir(), "iis")
	traffic, _ := ioutil.ReadFile(tmppath)
	if len(traffic) == 8 {
		Survey.Written = int64(binary.BigEndian.Uint64(traffic[:]))
	}

	go func() {
		last := Survey.Written
		for {
			time.Sleep(time.Minute)

			traffic := [8]byte{}
			binary.BigEndian.PutUint64(traffic[:], uint64(Survey.Written))

			if Survey.Written != last {
				// log.Println("[Traffic recorder] traffic:", Survey.Written, ioutil.WriteFile(tmppath, traffic[:], 0700))
			}

			last = Survey.Written
		}
	}()
}

var staticHeader = http.Header{
	"Content-Type": []string{"something"},
}

type fakeResponseCatcher struct {
	bytes.Buffer
}

func (w *fakeResponseCatcher) WriteHeader(code int) {}

func (w *fakeResponseCatcher) Header() http.Header { return staticHeader }

func RenderTemplateString(name string, v interface{}) string {
	p := fakeResponseCatcher{}
	engine.HTMLRender.Instance(name, v).Render(&p)
	return p.String()
}
