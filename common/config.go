package common

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"html/template"
	"io/ioutil"
	"net"
	"regexp"

	"github.com/ipipdotnet/ipdb-go"
)

var Cfg = struct {
	Key             string
	RPCKey          string
	Cooldown        int   // minute
	TokenTTL        int64 // minute
	IDTokenTTL      int64 // second
	MaxContent      int64 // byte
	MinContent      int64 // byte
	AdminName       string
	PostsPerPage    int
	MaxImagesCache  int // GB
	Domains         []string
	MediaDomain     string
	IPBlacklist     []string
	MaxMentions     int
	DyRegion        string
	CwRegion        string
	DyAccessKey     string
	DySecretKey     string
	S3AccessKey     string
	S3SecretKey     string
	S3Region        string
	S3Endpoint      string
	S3Bucket        string
	RedisAddr       string
	ReadOnly        bool
	IPIPDatabase    string
	HCaptchaSiteKey string
	HCaptchaSecKey  string

	// inited after Cfg being read
	Blk               cipher.Block
	KeyBytes          []byte
	IPBlacklistParsed []*net.IPNet
	IPIPDB            *ipdb.City
}{
	MediaDomain:     "/i",
	TokenTTL:        10,
	IDTokenTTL:      600,
	Key:             "0123456789abcdef",
	AdminName:       "zzzz",
	MaxContent:      4096,
	MinContent:      8,
	PostsPerPage:    30,
	Cooldown:        5,
	MaxMentions:     3,
	MaxImagesCache:  10,
	HCaptchaSiteKey: "10000000-ffff-ffff-ffff-000000000001",
	HCaptchaSecKey:  "0x0000000000000000000000000000000000000000",
}

func MustLoadConfig() {
	buf, err := ioutil.ReadFile("config.json")
	if err != nil {
		panic(err)
	}

	buf = regexp.
		MustCompile(`(?:/\*[^*]*\*+(?:[^/*][^*]*\*+)*/|//[^\n]*(?:\n|$))|("[^"\\]*(?:\\[\S\s][^"\\]*)*"|'[^'\\]*(?:\\[\S\s][^'\\]*)*'|[\S\s][^/"'\\]*)`).
		ReplaceAll(buf, []byte("$1"))

	if err := json.Unmarshal(buf, &Cfg); err != nil {
		panic(err)
	}

	Cfg.Blk, _ = aes.NewCipher([]byte(Cfg.Key))
	Cfg.KeyBytes = []byte(Cfg.Key)

	for _, addr := range Cfg.IPBlacklist {
		_, subnet, _ := net.ParseCIDR(addr)
		Cfg.IPBlacklistParsed = append(Cfg.IPBlacklistParsed, subnet)
	}

	if Cfg.IPIPDatabase != "" {
		db, err := ipdb.NewCity(Cfg.IPIPDatabase)
		if err != nil {
			panic(err)
		}
		Cfg.IPIPDB = db
	}
}

type CSSConfig struct {
	BodyBG            string // main background color
	InputBG           string
	Link              string
	Navbar            string
	NavbarBottom      string
	Border            string
	DarkBorder        string
	NormalText        string
	LightText         string
	MidGrayText       string
	LightBG           string
	Row               string
	RowHeader         string
	ClusterRowHeader  string
	FoobarHoverBottom string
	TextShadow        string
	ModText           string
	PostButton        string
	PostButtonHover   string
	DropdownItemHover string
	RedText           string
	GreenText         string
	OrangeText        string
	NSFWText          string
	InboxMessage      string
	AddFriend         string
	RemoveFriend      string
	Button            string
	ButtonDisabled    string
	ToastBG           string
	Toast             string

	Mode string
}

var CSSLightConfig = CSSConfig{
	InputBG:           "#fff",
	BodyBG:            "#fff",
	Button:            "rgb(var(--pure-material-primary-rgb, 33, 150, 243))",
	ButtonDisabled:    "rgba(var(--pure-material-onsurface-rgb, 0, 0, 0), 0.38)",
	Link:              "#2a66d9",
	Navbar:            "#feb",
	NavbarBottom:      "rgba(0,0,0,0.04)",
	Border:            "#ddd",
	DarkBorder:        "#ddd",
	NormalText:        "#233",
	LightText:         "#aaa",
	MidGrayText:       "#666",
	LightBG:           "#fafbfc",
	Row:               "#f6f6f6",
	RowHeader:         "rgba(0,0,0,0.04)",
	ClusterRowHeader:  "rgba(0,0,0,0.015)",
	FoobarHoverBottom: "#677",
	TextShadow:        "#677",
	ModText:           "#673ab7",
	PostButton:        "#64b5f6",
	PostButtonHover:   "#2196f3",
	DropdownItemHover: "#bdf",
	RedText:           "#f52",
	GreenText:         "#4a5",
	OrangeText:        "#f90",
	NSFWText:          "#bb7ab0",
	InboxMessage:      "#3f51b5",
	AddFriend:         "#098",
	RemoveFriend:      "#e16",
	ToastBG:           "rgba(0,0,0,0.9)",
	Toast:             "white",
}

var CSSDarkConfig = CSSConfig{
	Mode:              "dark",
	BodyBG:            "#1b2838",
	InputBG:           "#2a3f5a",
	Button:            "#67c1f5",
	ButtonDisabled:    "#666",
	Row:               "#0d131b",
	RowHeader:         "rgba(255,255,255,0.07)",
	ClusterRowHeader:  "rgba(255,255,255,0.03)",
	Navbar:            "#312244",
	NavbarBottom:      "#09080a",
	Border:            "#234456",
	DarkBorder:        "#093248",
	NormalText:        "#eee",
	LightBG:           "#192a40",
	DropdownItemHover: "rgba(255,255,255,0.15)",
	ModText:           "#fff59d",
	Link:              "#ff9800",
	LightText:         "#666",
	MidGrayText:       "#aaa",
	RemoveFriend:      "#F06292",
	PostButton:        "#488dc3",
	PostButtonHover:   "#176caf",
	ToastBG:           "rgba(255,255,255,0.9)",
	Toast:             "black",
	InboxMessage:      "#ffeb3b",

	FoobarHoverBottom: "#677",
	TextShadow:        "#677",
	RedText:           "#f52",
	GreenText:         "#4a5",
	OrangeText:        "#f90",
	NSFWText:          "#bb7ab0",
	AddFriend:         "#098",
}

func (c *CSSConfig) WriteTemplate(path string, t string) {
	tmpl, err := template.New("").Parse(t)
	if err != nil {
		panic(err)
	}
	p := &bytes.Buffer{}
	if err := tmpl.Execute(p, c); err != nil {
		panic(err)
	}
	if err := ioutil.WriteFile(path, p.Bytes(), 0777); err != nil {
		panic(err)
	}
}
