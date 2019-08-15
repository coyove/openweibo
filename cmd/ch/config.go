package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"io/ioutil"
	"regexp"

	"github.com/coyove/ch/driver"
	"github.com/coyove/common/lru"
	"gopkg.in/yaml.v2"
)

var (
	config = struct {
		Storages      []driver.StorageConfig `yaml:"Storages"`
		CacheSize     int64                  `yaml:"CacheSize"`
		Key           string                 `yaml:"Key"`
		TokenTTL      int64                  `yaml:"TokenTTL"`
		MaxContent    int64                  `yaml:"MaxContent"`
		MinContent    int64                  `yaml:"MinContent"`
		MaxTags       int64                  `yaml:"MaxTags"`
		AdminName     string                 `yaml:"AdminName"`
		PostsPerPage  int64                  `yaml:"PostsPerPage"`
		Tags          []string               `yaml:"Tags"`
		Domain        string                 `yaml:"Domain"`
		ImageDomain   string                 `yaml:"ImageDomain"`
		ImageDisabled bool                   `yaml:"ImageDisabled"`

		// inited after config being read
		blk           cipher.Block
		adminNameHash string
		publicString  string
	}{
		CacheSize:    1,
		TokenTTL:     1,
		Key:          "0123456789abcdef",
		AdminName:    "zzz",
		MaxContent:   4096,
		MinContent:   8,
		MaxTags:      4,
		PostsPerPage: 30,
		Tags:         []string{},
	}

	survey struct {
		render struct {
			avg int64
			max int64
		}
	}
)

func loadConfig() {
	buf, err := ioutil.ReadFile("config.yml")
	if err != nil {
		panic(err)
	}

	if err := yaml.Unmarshal(buf, &config); err != nil {
		panic(err)
	}

	dedup = lru.NewCache(1024)
	config.blk, _ = aes.NewCipher([]byte(config.Key))
	config.adminNameHash = authorNameToHash(config.AdminName)

	buf, _ = json.MarshalIndent(config, "<li>", "    ")
	buf = regexp.MustCompile(`(?i)".*(token|key|admin).+`).ReplaceAllFunc(buf, func(in []byte) []byte {
		return bytes.Repeat([]byte("\u2588"), len(in)/2+1)
	})
	config.publicString = "<li>" + string(buf)
}
