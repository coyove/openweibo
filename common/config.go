package common

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
	Key            string   `yaml:"Key"`
	RPCKey         string   `yaml:"RPCKey"`
	Cooldown       int      `yaml:"Cooldown"`   // minute
	TokenTTL       int64    `yaml:"TokenTTL"`   // minute
	IDTokenTTL     int64    `yaml:"IDTokenTTL"` // second
	MaxContent     int64    `yaml:"MaxContent"` // byte
	MinContent     int64    `yaml:"MinContent"` // byte
	AdminName      string   `yaml:"AdminName"`
	PostsPerPage   int      `yaml:"PostsPerPage"`
	MaxImagesCache int      `yaml:"MaxImagesCache"` // GB
	Domain         string   `yaml:"Domain"`
	IPBlacklist    []string `yaml:"IPBlacklist"`
	MaxMentions    int      `yaml:"MaxMentions"`
	DyRegion       string   `yaml:"DyRegion"`
	CwRegion       string   `yaml:"CwRegion"`
	DyAccessKey    string   `yaml:"DyAccessKey"`
	DySecretKey    string   `yaml:"DySecretKey"`
	RedisAddr      string   `yaml:"RedisAddr"`
	ReadOnly       bool     `yaml:"ReadOnly"`

	// inited after Cfg being read
	Blk               cipher.Block
	KeyBytes          []byte
	IPBlacklistParsed []*net.IPNet
	PublicString      string
	PrivateString     string
}{
	TokenTTL:       10,
	IDTokenTTL:     600,
	Key:            "0123456789abcdef",
	AdminName:      "zzzz",
	MaxContent:     4096,
	MinContent:     8,
	PostsPerPage:   30,
	Cooldown:       5,
	MaxMentions:    3,
	MaxImagesCache: 10,
}

func MustLoadConfig() {
	buf, err := ioutil.ReadFile("config.yml")
	if err != nil {
		panic(err)
	}

	if err := yaml.Unmarshal(buf, &Cfg); err != nil {
		panic(err)
	}

	Cfg.Blk, _ = aes.NewCipher([]byte(Cfg.Key))
	Cfg.KeyBytes = []byte(Cfg.Key)

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
