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

	"github.com/armon/go-radix"
)

var AsciiEmojisTree = func() *radix.Tree {
	r := radix.New()
	for _, e := range AsciiEmojis {
		r.Insert(e, nil)
	}
	return r
}()

var AsciiEmojis = []string{
	"|∀ﾟ",
	"(´ﾟДﾟ`)",
	"(;´Д`)",
	"(｀･ω･)",
	"(=ﾟωﾟ)=",
	"| ω・´)",
	"|-` )",
	"|д` )",
	"|ー` )",
	"|∀` )",
	"(つд⊂)",
	"(ﾟДﾟ≡ﾟДﾟ)",
	"(＾o＾)ﾉ",
	"(|||ﾟДﾟ)",
	"( ﾟ∀ﾟ)",
	"( ´∀`)",
	"(*´∀`)",
	"(*ﾟ∇ﾟ)",
	"(*ﾟーﾟ)",
	"(　ﾟ 3ﾟ)",
	"( ´ー`)",
	"( ・_ゝ・)",
	"( ´_ゝ`)",
	"(*´д`)",
	"(・ー・)",
	"(・∀・)",
	"(ゝ∀･)",
	"(〃∀〃)",
	"(*ﾟ∀ﾟ*)",
	"( ﾟ∀。)",
	"( `д´)",
	"(`ε´ )",
	"(`ヮ´ )",
	"σ`∀´)",
	" ﾟ∀ﾟ)σ",
	"ﾟ ∀ﾟ)ノ",
	"(╬ﾟдﾟ)",
	"(|||ﾟдﾟ)",
	"( ﾟдﾟ)",
	"Σ( ﾟдﾟ)",
	"( ;ﾟдﾟ)",
	"( ;´д`)",
	"(　д ) ﾟ ﾟ",
	"( ☉д⊙)",
	"(((　ﾟдﾟ)))",
	"( ` ・´)",
	"( ´д`)",
	"( -д-)",
	"(>д<)",
	"･ﾟ( ﾉд`ﾟ)",
	"( TдT)",
	"(￣∇￣)",
	"(￣3￣)",
	"(￣ｰ￣)",
	"(￣ . ￣)",
	"(￣皿￣)",
	"(￣艸￣)",
	"(￣︿￣)",
	"(￣︶￣)",
	"ヾ(´ωﾟ｀)",
	"(*´ω`*)",
	"(・ω・)",
	"( ´・ω)",
	"(｀・ω)",
	"(´・ω・`)",
	"(`・ω・´)",
	"( `_っ´)",
	"( `ー´)",
	"( ´_っ`)",
	"( ´ρ`)",
	"( ﾟωﾟ)",
	"(oﾟωﾟo)",
	"(　^ω^)",
	"(｡◕∀◕｡)",
	"/( ◕‿‿◕ )\\",
	"ヾ(´ε`ヾ)",
	"(ノﾟ∀ﾟ)ノ",
	"(σﾟдﾟ)σ",
	"(σﾟ∀ﾟ)σ",
	"|дﾟ )",
	"┃電柱┃",
	"ﾟ(つд`ﾟ)",
	"ﾟÅﾟ )",
	"⊂彡☆))д`)",
	"⊂彡☆))д´)",
	"⊂彡☆))∀`)",
	"(´∀((☆ミつ",
}

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
	MaxRequestSize  int // MB
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
	HCaptchaSiteKey string
	HCaptchaSecKey  string
	SMTPServer      string
	SMTPEmail       string
	SMTPPassword    string

	// inited after Cfg being read
	Blk               cipher.Block
	KeyBytes          []byte
	IPBlacklistParsed []*net.IPNet
}{
	MediaDomain:     "/i",
	TokenTTL:        10,
	IDTokenTTL:      600,
	Key:             "0123456789abcdef",
	AdminName:       "zzzz",
	MaxContent:      1024,
	MinContent:      8,
	PostsPerPage:    30,
	Cooldown:        5,
	MaxMentions:     3,
	MaxRequestSize:  6,
	HCaptchaSiteKey: "10000000-ffff-ffff-ffff-000000000001",
	HCaptchaSecKey:  "0x0000000000000000000000000000000000000000",
}

func MustLoadConfig(path string) {
	buf, err := ioutil.ReadFile(path)
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
	BodyBG         string // main background color
	ContainerBG    string
	InputBG        string
	Navbar         string
	NavbarTitlebar string
	Border         string
	NormalText     string
	MidGrayText    string
	LightBG        string
	Row            string
	ModText        string
	RedText        string
	GreenText      string
	NSFWText       string
	RemoveFriend   string
	Button         string
	ButtonDisabled string
	ToastBG        string
	Toast          string

	Mode string
}

var CSSLightConfig = CSSConfig{
	InputBG:        "#fff",
	BodyBG:         "#dadada",
	ContainerBG:    "#fff",
	Button:         "#759cd8",
	ButtonDisabled: "rgba(var(--pure-material-onsurface-rgb, 0, 0, 0), 0.38)",
	Navbar:         "#a9c9c9",
	NavbarTitlebar: "#e6e6e6",
	Border:         "#ddd",
	NormalText:     "#233",
	MidGrayText:    "#666",
	LightBG:        "#fafbfc",
	Row:            "#f0f0f0",
	ModText:        "#673ab7",
	RedText:        "#f52",
	GreenText:      "#b88855",
	RemoveFriend:   "#e16",
	NSFWText:       "#bb7ab0",
	ToastBG:        "rgba(0,0,0,0.9)",
	Toast:          "white",
}

var CSSDarkConfig = CSSConfig{
	Mode:           "dark",
	BodyBG:         "#273a50",
	ContainerBG:    "#1b2838",
	InputBG:        "#2a3f5a",
	Button:         "#67c1f5",
	ButtonDisabled: "#666",
	Row:            "#161c26",
	Navbar:         "#402040",
	NavbarTitlebar: "#281a28",
	Border:         "#234456",
	NormalText:     "#eee",
	LightBG:        "#192a40",
	ModText:        "#fff59d",
	MidGrayText:    "#aaa",
	RemoveFriend:   "#F06292",
	ToastBG:        "rgba(255,255,255,0.9)",
	Toast:          "black",
	RedText:        "#f52",
	GreenText:      "#4a5",
	NSFWText:       "#bb7ab0",
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
