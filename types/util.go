package types

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"html"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coyove/common/dejavu"
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

var NotFoundPNG []byte

func WriteImageText(w io.Writer, texts ...string) {
	if w, ok := w.(*Response); ok {
		w.Header().Add("Content-Type", "image/png")
	}
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	draw.Draw(img, img.Bounds(), image.Black, image.Point{}, draw.Over)

	ln := 0
	for _, text := range texts {
		ln += int(math.Ceil(float64(len(text)) / 28))
	}

	y := (200-ln*dejavu.FullHeight)/2 + dejavu.FullHeight
	for _, text := range texts {
		for i := 0; i < len(text); i += 28 {
			end := i + 28
			if end > len(text) {
				dejavu.DrawText(img, text[i:], (200-len(text[i:])*dejavu.Width)/2, y, image.White)
			} else {
				dejavu.DrawText(img, text[i:end], 2, y, image.White)
			}
			y += dejavu.FullHeight
		}
	}

	png.Encode(w, img)
}
