package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"html/template"
	"io/ioutil"
	"net"
	"regexp"

	"github.com/coyove/common/lru"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v2"
)

var (
	config = struct {
		CacheSize     int64    `yaml:"CacheSize"`
		Key           string   `yaml:"Key"`
		TokenTTL      int64    `yaml:"TokenTTL"`
		MaxContent    int64    `yaml:"MaxContent"`
		MinContent    int64    `yaml:"MinContent"`
		MaxTags       int64    `yaml:"MaxTags"`
		AdminName     string   `yaml:"AdminName"`
		PostsPerPage  int      `yaml:"PostsPerPage"`
		Tags          []string `yaml:"Tags"`
		Domain        string   `yaml:"Domain"`
		ImageDomain   string   `yaml:"ImageDomain"`
		ImageDisabled bool     `yaml:"ImageDisabled"`
		InboxSize     int      `yaml:"InboxSize"`
		IPBlacklist   []string `yaml:"IPBlacklist"`
		Cooldown      int      `yaml:"Cooldown"`

		// inited after config being read
		blk           cipher.Block
		adminNameHash string
		ipblacklist   []*net.IPNet
		tagsMap       map[string]bool

		publicString  string
		privateString string
	}{
		CacheSize:    1,
		TokenTTL:     1,
		Key:          "0123456789abcdef",
		AdminName:    "zzz",
		MaxContent:   4096,
		MinContent:   8,
		MaxTags:      4,
		PostsPerPage: 7,
		Tags:         []string{},
		InboxSize:    100,
		Cooldown:     10,
	}

	survey struct {
		render struct {
			avg int64
			max int64
		}
		written int64
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
	config.tagsMap = map[string]bool{}

	for _, tag := range config.Tags {
		config.tagsMap[tag] = true
	}

	for _, addr := range config.IPBlacklist {
		_, subnet, _ := net.ParseCIDR(addr)
		config.ipblacklist = append(config.ipblacklist, subnet)
	}

	buf, _ = json.MarshalIndent(config, "<li>", "    ")
	config.privateString = "<li>" + string(buf)
	buf = regexp.MustCompile(`(?i)".*(token|key|admin).+`).ReplaceAllFunc(buf, func(in []byte) []byte {
		return bytes.Repeat([]byte("\u2588"), len(in)/2+1)
	})
	config.publicString = "<li>" + string(buf)
}

func handleCurrentStat(g *gin.Context) {
	p := struct{ Config template.HTML }{Config: template.HTML(config.publicString)}
	if isAdmin(g) {
		p.Config = template.HTML(config.privateString)
	}
	g.HTML(200, "stat.html", p)
}

func handleTags(g *gin.Context) {
	g.HTML(200, "tags.html", struct{ Tags map[string]int }{m.TagsCount(config.Tags...)})
}
