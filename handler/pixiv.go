package handler

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var eririImages = [][2][]byte{}

func Eriri(g *gin.Context) {
	x := eririImages
	if len(x) == 0 {
		g.Status(404)
		return
	}
	y := x[rand.Intn(len(x))]
	g.Writer.Header().Add("Content-Type", "image/jpeg")
	g.Writer.Header().Add("Cache-Control", "max-age=600")
	g.Writer.Write(y[1])
}

func GetEriri(word string) {
	start := time.Now()

	c := &http.Client{Timeout: time.Second * 5}
	resp, err := c.Get("https://www.pixiv.net/ajax/search/artworks/" + word + "?order=date_d&mode=safe&p=1&s_mode=s_tag_full&type=all")
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()
	buf, _ := ioutil.ReadAll(resp.Body)

	m := map[string]interface{}{}
	json.Unmarshal(buf, &m)

	var data []map[string]interface{}
	func() {
		defer func() { recover() }()
		for _, d := range m["body"].(map[string]interface{})["illustManga"].(map[string]interface{})["data"].([]interface{}) {
			data = append(data, d.(map[string]interface{}))
		}
	}()

	download := func(url string) ([]byte, error) {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Add("Referer", "https://www.pixiv.net/tags/"+word+"/artworks?mode=safe")
		resp, err := c.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return ioutil.ReadAll(resp.Body)
	}

	{
		wg := sync.WaitGroup{}
		m, mu, msize := [][2][]byte{}, sync.Mutex{}, 0
		for _, d := range data {
			pid, _ := d["illustId"].(string)
			url, _ := d["url"].(string)
			if url == "" {
				continue
			}
			wg.Add(1)
			go func() {
				buf, err := download(url)
				if err == nil {
					mu.Lock()
					m = append(m, [2][]byte{[]byte(pid), buf})
					msize += len(buf)
					mu.Unlock()
				}
				wg.Done()
			}()
		}
		wg.Wait()

		eririImages = m

		log.Println("eriri worker", time.Since(start), msize/1024)
	}

	time.AfterFunc(time.Minute*30, func() { GetEriri(word) })
}
