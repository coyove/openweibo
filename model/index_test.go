package model

import (
	"log"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coyove/iis/ik"
)

func TestSearch(t *testing.T) {
	rand.Seed(time.Now().Unix())
	var text string
	for i := 0; i < 5e5; i++ {
		id := ik.NewGeneralID()
		text = strings.Repeat(strconv.FormatInt(rand.Int63(), 36), 10)
		Index("test", id, text)
		if i%1e4 == 0 {
			log.Println(i)
		}
	}
	t.Log(Search("test", text[10:rand.Intn(len(text)/2)+10], 0, 10))
}
