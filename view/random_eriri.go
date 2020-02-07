package view

import (
	"bytes"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var eririGallery struct {
	sync.RWMutex
	data []string
}

func getEririWorker() {
	resp, err := http.Get("https://danbooru.donmai.us/posts?tags=sawamura_spencer_eriri")
	if err != nil {
		log.Println("Eriri:", err)
		return
	}
	defer resp.Body.Close()

	eririGallery.Lock()
	defer eririGallery.Unlock()

	eririGallery.data = []string{}

	re := regexp.MustCompile(`<img.+?src=(\S+).+?>`)
	buf, _ := ioutil.ReadAll(resp.Body)
	for _, m := range re.FindAllSubmatch(buf, -1) {
		x := m[1]
		if len(x) < 2 {
			continue
		}
		if !bytes.Contains(x, []byte("/preview/")) {
			continue
		}
		x = x[1 : len(x)-1]

		eririGallery.data = append(eririGallery.data, string(x))
	}
}

func init() {
	go func() {
		for {
			getEririWorker()
			time.Sleep(time.Minute * 10)
		}
	}()
}

func RandomEririImage(g *gin.Context) {
	eririGallery.RLock()
	defer eririGallery.RUnlock()

	if len(eririGallery.data) == 0 {
		g.Status(404)
		return
	}

	g.Redirect(302, eririGallery.data[rand.Intn(len(eririGallery.data))])
}
