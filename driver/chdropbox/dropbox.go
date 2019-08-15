package chdropbox

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/common/sched"
	"github.com/coyove/iis/driver"
)

var rxFn = regexp.MustCompile(`[^a-zA-Z0-9\.]`)

func NewNode(name string, config driver.StorageConfig) *driver.Node {
	n := &driver.Node{
		KV: &Storage{
			accessToken: config.AccessToken,
			client:      &http.Client{},
			throt:       driver.NewTokenBucket(config.Throt),
			config:      config,
		},
		Name:   name,
		Weight: config.Weight,
	}
	if n.Weight <= 0 {
		panic(n.Weight)
	}
	if err := n.KV.(*Storage).checkSpace(); err != nil {
		n.KV.(*Storage).config.Offline = true
	}
	return n
}

type Storage struct {
	accessToken string
	throt       *driver.TokenBucket
	client      *http.Client
	config      driver.StorageConfig
	stat        struct {
		ping       int64
		lastUpdate time.Time
		Used       int64 `json:"used"`
		Allocation struct {
			Allocated int64 `json:"allocated"`
		} `json:"allocation"`
	}
}

func sanitize(k string) string {
	k = rxFn.ReplaceAllString(k, "_")
	return k
}

func (s *Storage) newReq(path string, r io.Reader) *http.Request {
	req, _ := http.NewRequest("POST", path, r)
	req.Header.Add("Authorization", "Bearer "+s.accessToken)
	return req
}

func (s *Storage) doReq(req *http.Request) (map[string]interface{}, error) {
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

func (s *Storage) calcPath(k string) (string, string) {
	k = sanitize(k)
	h := sha1.Sum([]byte(k))
	path := fmt.Sprintf("/ch/%x/%x/%s", h[0], h[1], k)
	return path, `{"path":"` + path + `"}`
}

func (s *Storage) Put(k string, v []byte) error {
	path, _ := s.calcPath(k)

	req := s.newReq("https://content.dropboxapi.com/2/files/upload", bytes.NewReader(v))
	req.Header.Add("Content-Type", "application/octet-stream")
	req.Header.Add("Dropbox-API-Arg",
		fmt.Sprintf(`{"path":"%s","mode":"add","autorename":false,"mute":false,"strict_conflict":false}`, path))

	m, err := s.doReq(req)
	if err != nil {
		return err
	}

	if id, _ := m["id"].(string); id == "" {
		return fmt.Errorf("failed to put %s, error: %s", k, string(m["_ch_raw"].([]byte)))
	}

	return nil
}

func (s *Storage) Get(k string) ([]byte, error) {
	_, jpath := s.calcPath(k)

	req := s.newReq("https://content.dropboxapi.com/2/files/download", nil)
	req.Header.Add("Content-Type", "application/octet-stream")
	req.Header.Add("Dropbox-API-Arg", jpath)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	m := map[string]interface{}{}
	if err := json.Unmarshal([]byte(resp.Header.Get("Dropbox-API-Result")), &m); err == nil {
		if id, _ := m["id"].(string); id != "" {
			size, _ := strconv.Atoi(fmt.Sprint(m["size"]))
			if !s.throt.Consume(int64(size)) {
				return nil, driver.ErrThrottled
			}
			buf, _ := ioutil.ReadAll(resp.Body)
			return buf, nil
		}
	}

	return nil, driver.ErrKeyNotFound
}

func (s *Storage) Delete(k string) error {
	_, jpath := s.calcPath(k)
	req := s.newReq("https://api.dropboxapi.com/2/files/delete_v2", strings.NewReader(jpath))
	req.Header.Add("Content-Type", "application/json")
	m, err := s.doReq(req)
	if err != nil {
		return err
	}
	if m, _ := m["metadata"].(map[string]interface{}); m != nil {
		if id, _ := m["id"]; id != "" {
			return nil
		}
	}
	return fmt.Errorf("failed to delete %s, error: %s", k, string(m["_ch_raw"].([]byte)))
}

func (s *Storage) Stat() driver.Stat {
	return driver.Stat{
		UpdateTime: s.stat.lastUpdate,
		Ping:       s.stat.ping,
		Throt:      s.throt.String(),
	}
}

func (s *Storage) Space() (bool, int64, int64) {
	return s.config.Offline, s.stat.Allocation.Allocated, s.stat.Used
}

func (s *Storage) checkSpace() error {
	start := time.Now()
	req := s.newReq("https://api.dropboxapi.com/2/users/get_space_usage", nil)
	resp, err := s.client.Do(req)
	if err != nil {
		log.Println("[dropbox]", err)
		return err
	}

	buf, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	json.Unmarshal(buf, &s.stat)

	s.stat.ping = time.Since(start).Nanoseconds() / 1e6
	s.stat.lastUpdate = time.Now()

	sched.Schedule(func() {
		go s.checkSpace()
	}, time.Second*time.Duration(20+rand.Intn(50)))

	return nil
}
