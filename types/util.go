package types

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"html"
	"math"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"unsafe"

	nh "golang.org/x/net/html"
)

func LocalTime(v time.Time) time.Time {
	return v.UTC().Add(time.Duration(Config.TzOffset) * time.Hour)
}

func Uint64Bytes(v uint64) []byte {
	var p [8]byte
	binary.BigEndian.PutUint64(p[:], uint64(v))
	return p[:]
}

func BytesUint64(p []byte) uint64 {
	if len(p) == 0 {
		return 0
	}
	return binary.BigEndian.Uint64(p)
}

func Float64Bytes(v float64) []byte {
	var p [8]byte
	binary.BigEndian.PutUint64(p[:], math.Float64bits(v))
	return p[:]
}

func BytesFloat64(p []byte) float64 {
	if len(p) == 0 {
		return 0
	}
	return math.Float64frombits(binary.BigEndian.Uint64(p))
}

func UUIDStr() string {
	uuid := make([]byte, 48)
	rand.Read(uuid[:16])
	hex.Encode(uuid[16:], uuid[:16])
	return string(uuid[16:])
}

func DedupUint64(v []uint64) []uint64 {
	if len(v) <= 1 {
		return v
	}
	if len(v) == 2 {
		if v[0] == v[1] {
			return v[:1]
		}
		return v
	}
	s := uint64Sort{v}
	sort.Sort(s)
	for i := len(v) - 1; i > 0; i-- {
		if v[i] == v[i-1] {
			v = append(v[:i], v[i+1:]...)
		}
	}
	return v
}

func ContainsUint64(a []uint64, b uint64) bool {
	if len(a) < 10 {
		for _, v := range a {
			if v == b {
				return true
			}
		}
		return false
	}
	s := uint64Sort{a}
	if !sort.IsSorted(s) {
		sort.Sort(s)
	}
	idx := sort.Search(len(a), func(i int) bool { return a[i] >= b })
	return idx < len(a) && a[idx] == b
}

func EqualUint64(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Slice(a, func(i, j int) bool { return a[i] < a[j] })
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func UTF16Trunc(v string, max int) string {
	src, sz := v, 0
	for len(v) > 0 && sz < max {
		r, n := utf8.DecodeRuneInString(v)
		if n == 0 {
			break
		}
		if r > 65535 {
			sz += 2
		} else {
			sz++
		}
		v = v[n:]
	}
	return src[:len(src)-len(v)]
}

func UTF16LenExceeds(v string, max int) bool {
	if len(v) < max {
		return false
	}
	for sz := 0; len(v) > 0; {
		r, n := utf8.DecodeRuneInString(v)
		if n == 0 {
			break
		}
		if r > 65535 {
			sz += 2
		} else {
			sz++
		}
		if sz > max {
			return true
		}
		v = v[n:]
	}
	return false
}

func RemoteIPv4(r *http.Request) net.IP {
	xff := r.Header.Get("X-Forwarded-For")
	ips := strings.Split(xff, ",")
	for _, ip := range ips {
		p := net.ParseIP(strings.TrimSpace(ip))
		if p != nil {
			return p.To4()
		}
		break
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	p := net.ParseIP(ip)
	if p != nil {
		return p.To4()
	}
	return net.IP{0, 0, 0, 0}
}

func Uint64Hash(v uint64) uint64 {
	const offset64 = 14695981039346656037
	const prime64 = 1099511628211
	var h uint64 = offset64
	for i := 0; i < 64; i += 8 {
		h *= prime64
		h ^= v >> i
	}
	return h
}

func SafeHTML(v string) string {
	return html.EscapeString(v)
}

func JoinUint64List(ids []uint64) string {
	var tmp []string
	for _, id := range ids {
		tmp = append(tmp, strconv.FormatUint(id, 10))
	}
	return strings.Join(tmp, ",")
}

func SplitUint64List(v string) (ids []uint64) {
	for _, p := range strings.Split(v, ",") {
		id, _ := strconv.ParseUint(p, 10, 64)
		if id > 0 {
			ids = append(ids, (id))
		}
	}
	return
}

var regClip = regexp.MustCompile(`(https?://[^\s]+|<|>)`)

func unq(v string) string {
	x, err := url.QueryUnescape(v)
	if err != nil {
		return v
	}
	return x
}

func RenderClip(v string) string {
	out := &bytes.Buffer{}
	return regClip.ReplaceAllStringFunc(v, func(in string) string {
		if in == "<" {
			return "&lt;"
		}
		if in == ">" {
			return "&gt;"
		}

		prefix := "http://"
		if strings.HasPrefix(in, "https://") {
			prefix = "https://"
		}
		switch rest := in[len(prefix):]; {
		case strings.HasPrefix(rest, "text:"):
			return unq(rest[5:])
		case strings.HasPrefix(rest, "section:"):
			return "<h2 style='display:inline'>" + unq(rest[8:]) + "</h2>"
		case strings.HasPrefix(rest, "u:") || strings.HasPrefix(rest, "i:") || strings.HasPrefix(rest, "b:"):
			return fmt.Sprintf("<%c>%s</%c>", rest[0], unq(rest[2:]), rest[0])
		case strings.HasPrefix(rest, "hl:"):
			return "<b class=highlight>" + unq(rest[3:]) + "</b>"
		case strings.HasPrefix(rest, "!"):
			out.Reset()
			z := nh.NewTokenizer(strings.NewReader(unq(rest[1:])))
			for {
				tt := z.Next()
				if tt == nh.ErrorToken {
					break
				} else if tt == nh.StartTagToken || tt == nh.EndTagToken {
					tag, _ := z.TagName()
					switch ts := *(*string)(unsafe.Pointer(&tag)); ts {
					case "b", "pre", "code", "div", "p", "span", "u", "i", "hr", "br", "strong":
						if tt == nh.EndTagToken {
							out.WriteString("</")
							out.Write(tag)
							out.WriteByte('>')
							if ts == "code" {
								out.WriteString("</div></div>")
							}
						} else {
							if ts == "code" {
								out.WriteString("<div style='position:relative;'><div style='white-space:pre;overflow-x:auto'>")
							}
							out.WriteByte('<')
							out.Write(tag)
							for {
								k, v, more := z.TagAttr()
								if *(*string)(unsafe.Pointer(&k)) == "style" {
									out.WriteString(" style='")
									out.Write(v)
									out.WriteByte('\'')
									break
								}
								if !more {
									break
								}
							}
							out.WriteByte('>')
						}
					default:
						continue
					}
				} else {
					out.Write(z.Raw())
				}
			}
			return out.String()
		case strings.HasPrefix(rest, "title:"):
			if idx := strings.IndexByte(rest, '/'); idx >= 0 {
				title := unq(rest[6:idx])
				url := rest[idx+1:]
				if strings.HasPrefix(url, "rel:") {
					return "<a href='" + url[4:] + "'>" + title + "</a>"
				}
				return "<a href='" + prefix + url + "'>" + title + "</a>"
			}
		case strings.HasPrefix(rest, "img:"):
			if idx := strings.IndexByte(rest, '/'); idx >= 0 {
				tag := reflect.StructTag(unq(rest[4:idx]))
				var width, height int
				fmt.Sscanf(tag.Get("size"), "%dx%d", &width, &height)
				if height <= 0 {
					height = 200
				}
				if width <= 0 {
					width = 200
				}
				url := prefix + rest[idx+1:]
				a := tag.Get("href")
				if a == "" {
					a = url
				}
				return fmt.Sprintf("<a href='%s' target=_blank>"+
					"<img src='%s' style='max-width:%dpx;width:100%%;max-height:%dpx;height:100%%;display:block'>"+
					"</a>", a, url, width, height)
			}
		case strings.HasPrefix(rest, "search:"):
		}
		return "<a href='" + in + "'>" + in + "</a>"
	})
}

func CleanTitle(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Replace(v, "//", "/", -1)
	v = strings.Replace(v, "\n", "", -1)
	v = strings.Replace(v, "\r", "", -1)
	v = strings.Trim(v, "/")
	v = UTF16Trunc(v, 1000) // hard limit
	return v
}

func UnescapeSpace(v string) string {
	if !strings.Contains(v, "\\") {
		return v
	}
	buf := &bytes.Buffer{}
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' {
			if i == len(v)-1 {
				buf.WriteByte('\\')
				break
			} else {
				buf.WriteByte(v[i+1])
				i++
			}
			continue
		}
		buf.WriteByte(v[i])
	}
	return buf.String()
}

type uint64Sort struct {
	data []uint64
}

func (h uint64Sort) Len() int {
	return len(h.data)
}

func (h uint64Sort) Less(i, j int) bool {
	return h.data[i] < h.data[j]
}

func (h uint64Sort) Swap(i, j int) {
	h.data[i], h.data[j] = h.data[j], h.data[i]
}
