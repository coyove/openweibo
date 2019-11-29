package config

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"io/ioutil"
	"net"
	"regexp"

	"gopkg.in/yaml.v2"
)

var Cfg = struct {
	Key          string   `yaml:"Key"`
	TokenTTL     int64    `yaml:"TokenTTL"`
	IDTokenTTL   int64    `yaml:"IDTokenTTL"`
	MaxContent   int64    `yaml:"MaxContent"`
	MinContent   int64    `yaml:"MinContent"`
	AdminName    string   `yaml:"AdminName"`
	PostsPerPage int      `yaml:"PostsPerPage"`
	Tags         []string `yaml:"Tags"`
	Domain       string   `yaml:"Domain"`
	IPBlacklist  []string `yaml:"IPBlacklist"`
	Cooldown     int      `yaml:"Cooldown"`
	NeedID       bool     `yaml:"NeedID"`
	MaxMentions  int      `yaml:"MaxMentions"`
	DyRegion     string   `yaml:"DyRegion"`
	CwRegion     string   `yaml:"CwRegion"`
	DyAccessKey  string   `yaml:"DyAccessKey"`
	DySecretKey  string   `yaml:"DySecretKey"`

	// inited after config being read
	Blk               cipher.Block
	KeyBytes          []byte
	IPBlacklistParsed []*net.IPNet
	TagsMap           map[string]bool
	PublicString      string
	PrivateString     string
}{
	TokenTTL:     1,
	IDTokenTTL:   600,
	Key:          "0123456789abcdef",
	AdminName:    "zzzz",
	MaxContent:   4096,
	MinContent:   8,
	PostsPerPage: 30,
	Tags:         []string{},
	Cooldown:     10,
	MaxMentions:  2,
}

func MustLoad() {
	buf, err := ioutil.ReadFile("config.yml")
	if err != nil {
		panic(err)
	}

	if err := yaml.Unmarshal(buf, &Cfg); err != nil {
		panic(err)
	}

	Cfg.Blk, _ = aes.NewCipher([]byte(Cfg.Key))
	Cfg.KeyBytes = []byte(Cfg.Key)
	Cfg.TagsMap = map[string]bool{}

	for _, tag := range Cfg.Tags {
		Cfg.TagsMap[tag] = true
	}

	for _, addr := range Cfg.IPBlacklist {
		_, subnet, _ := net.ParseCIDR(addr)
		Cfg.IPBlacklistParsed = append(Cfg.IPBlacklistParsed, subnet)
	}

	RegenConfigString()
}

func RegenConfigString() {
	Cfg.PrivateString = ""
	Cfg.PublicString = ""

	buf, _ := json.MarshalIndent(Cfg, "", "")
	buf = buf[1 : len(buf)-1]
	Cfg.PrivateString = string(buf)

	buf = regexp.MustCompile(`(?i)".*(token|dy|key|admin).+`).ReplaceAllFunc(buf, func(in []byte) []byte {
		return bytes.Repeat([]byte("\u2588"), len(in)/2+1)
	})
	Cfg.PublicString = string(buf)
}
