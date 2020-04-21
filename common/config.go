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

}

type CSSConfig struct {
	BodyBG            string // main background color
	InputBG           string
	Link              string
	Navbar            string
	Border            string
	NormalText        string
	LightText         string
	MidGrayText       string
	LightBG           string
	Row               string
	RowHeader         string
	FoobarHoverBottom string
	TextShadow        string
	ModText           string
	PostButton        string
	PostButtonHover   string
	DropdownItemHover string
	RedText           string
	GreenText         string
	OrangeText        string
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
	Border:            "#ddd",
	NormalText:        "#233",
	LightText:         "#aaa",
	MidGrayText:       "#666",
	LightBG:           "#fafbfc",
	Row:               "#f6f6f6",
	RowHeader:         "rgba(0,0,0,0.04)",
	FoobarHoverBottom: "#677",
	TextShadow:        "#677",
	ModText:           "#673ab7",
	PostButton:        "#64b5f6",
	PostButtonHover:   "#2196f3",
	DropdownItemHover: "#bdf",
	RedText:           "#f52",
	GreenText:         "#4a5",
	OrangeText:        "#f90",
	InboxMessage:      "#3f51b5",
	AddFriend:         "#098",
	RemoveFriend:      "#e16",
	ToastBG:           "rgba(0,0,0,0.5)",
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
	Navbar:            "#121923",
	Border:            "#234456",
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
	ToastBG:           "rgba(255,255,255,0.5)",
	Toast:             "black",
	InboxMessage:      "inherit; text-shadow: 0 0 8px rgba(0,0,0,0.5);",

	FoobarHoverBottom: "#677",
	TextShadow:        "#677",
	RedText:           "#f52",
	GreenText:         "#4a5",
	OrangeText:        "#f90",
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
