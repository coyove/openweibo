package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Dropbox struct {
	throt       *TokenBucket
	client      *http.Client
	accessToken string
	name        string
}

func NewDropbox(name string, accessToken string, throt string) *Dropbox {
	n := &Dropbox{
		accessToken: accessToken,
		throt:       NewTokenBucket(throt),
		name:        name,
		client: &http.Client{
			Timeout: time.Second * 10,
		},
	}
	return n
}

func (s *Dropbox) newReq(path string, r io.Reader) *http.Request {
	req, _ := http.NewRequest("POST", path, r)
	req.Header.Add("Authorization", "Bearer "+s.accessToken)
	return req
}

func (s *Dropbox) doReq(req *http.Request) (map[string]interface{}, error) {
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	buf, _ := ioutil.ReadAll(resp.Body)

	m := map[string]interface{}{}
	if err := json.Unmarshal(buf, &m); err != nil {
		return nil, err
	}

	m["_ch_raw"] = buf
	m["_ch_resp"] = resp
	return m, nil
}

func (s *Dropbox) calcPath(k string) (string, string) {
	buf, h := []byte(k), 0
	for i, c := range buf {
		if c := rune(c); !unicode.IsDigit(c) && !unicode.IsLetter(c) {
			buf[i] = '_'
		}
		if i == 64 {
			buf = buf[:64]
			break
		}
		h = h*31 + int(c)
	}
	path := fmt.Sprintf("/ch/%02x/%s", byte(h), string(buf))
	return path, `{"path":"` + path + `"}`
}

func (s *Dropbox) Put(k string, v []byte) error {
	path, _ := s.calcPath(k)

	req := s.newReq("https://content.dropboxapi.com/2/files/upload", bytes.NewReader(v))
	req.Header.Add("Content-Type", "application/octet-stream")
	req.Header.Add("Dropbox-API-Arg",
		fmt.Sprintf(`{"path":"%s","mode":"add","autorename":false,"mute":false,"strict_conflict":false}`, path))

	m, err := s.doReq(req)
	if err != nil {
		log.Println("[Dropbox]", s.name, "put err:", err)
		return ErrDead
	}

	if id, _ := m["id"].(string); id == "" {
		log.Println("[Dropbox]", s.name, "put resp:", string(m["_ch_raw"].([]byte)))
		return ErrDead
	}
	return ErrOK
}

func (s *Dropbox) Get(k string) ([]byte, error) {
	_, jpath := s.calcPath(k)

	req := s.newReq("https://content.dropboxapi.com/2/files/download", nil)
	req.Header.Add("Content-Type", "application/octet-stream")
	req.Header.Add("Dropbox-API-Arg", jpath)

	resp, err := s.client.Do(req)
	if err != nil {
		log.Println("[Dropbox]", s.name, "get err:", err)
		return nil, ErrDead
	}
	defer resp.Body.Close()

	m := map[string]interface{}{}
	if err := json.Unmarshal([]byte(resp.Header.Get("Dropbox-API-Result")), &m); err == nil {
		if id, _ := m["id"].(string); id != "" {
			size, _ := strconv.Atoi(fmt.Sprint(m["size"]))
			if !s.throt.Consume(int64(size)) {
				return nil, ErrThrottled
			}
			buf, _ := ioutil.ReadAll(resp.Body)
			return buf, ErrOK
		}
	}

	return nil, ErrNotFound
}

func (s *Dropbox) Delete(k string) error {
	_, jpath := s.calcPath(k)
	req := s.newReq("https://api.dropboxapi.com/2/files/delete_v2", strings.NewReader(jpath))
	req.Header.Add("Content-Type", "application/json")
	m, err := s.doReq(req)
	if err != nil {
		log.Println("[Dropbox]", s.name, "delete err:", err)
		return ErrDead
	}

	if m, _ := m["metadata"].(map[string]interface{}); m != nil {
		if id, _ := m["id"]; id != "" {
			return ErrOK
		}
	}

	log.Println("[Dropbox]", s.name, "delete resp:", string(m["_ch_raw"].([]byte)))
	return ErrDead
}
