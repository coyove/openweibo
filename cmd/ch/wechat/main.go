package main

import (
	"encoding/xml"
	"io/ioutil"
	"log"
	"net/http"
)

type msg struct {
	ToUserName   string `xml:"ToUserName"`
	FromUserName string `xml:"FromUserName"`
	CreateTime   int64  `xml:"CreateTime"`
	MsgId        uint64 `xml:"MsgId"`
	Content      string `xml:"Content"`
}

func main() {
	http.HandleFunc("/wechat_msg", func(w http.ResponseWriter, r *http.Request) {
		if echo := r.URL.Query().Get("echostr"); echo != "" {
			w.WriteHeader(200)
			w.Write([]byte(echo))
			return
		}

		buf, _ := ioutil.ReadAll(r.Body)
		m := msg{}
		xml.Unmarshal(buf, &m)

		if m.MsgId == 0 {
			w.WriteHeader(400)
			log.Println("Bad request:", r, "[", string(buf), "]")
			return
		}

		w.WriteHeader(200)
		log.Println(m.Content)
	})
	http.ListenAndServe(":80", nil)
}
