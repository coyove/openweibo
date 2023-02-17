package types

import (
	"bytes"
	"strings"
	"unicode"
	"unsafe"

	"golang.org/x/net/html"
)

type escaper struct{ bytes.Buffer }

func (e *escaper) writeEscape(p []byte) (n int, err error) {
	o := &e.Buffer
	for i := range p {
		switch p[i] {
		case '<':
			o.WriteString("&lt;")
			n += 4
		case '>':
			o.WriteString("&gt;")
			n += 4
		case '&':
			o.WriteString("&amp;")
			n += 5
		default:
			o.WriteByte(p[i])
			n++
		}
	}
	return
}

var allowedAttrs = map[string]bool{
	"id": true, "name": true, "for": true, "style": true,
	"title": true, "src": true, "alt": true, "class": true,
	"href": true, "width": true, "height": true, "target": true,
}

var allowedTags = func() map[string]bool {
	m := map[string]bool{}
	for _, p := range strings.Split(`b,em,pre,a,img,div,p,span,u,i,hr,br,strong,blockquote,video,code,ol,ul,li,
    table,tr,tbody,thead,td,th,h1,h2,h3,h4,label,font,textarea,input,sup,sub,dd,dl,dt`, ",") {
		m[strings.TrimSpace(p)] = true
	}
	return m
}()

func (out *escaper) writeEnd(tag string) {
	out.WriteString("</")
	out.WriteString(tag)
	out.WriteString(">")
	if tag == "code" {
		out.WriteString("</div></div>")
	}
}

func (out *escaper) writeStart(z *html.Tokenizer, tag string) {
	if tag == "code" {
		out.WriteString("<div style='position:relative;'><div style='white-space:pre;overflow-x:auto'><code")
	} else {
		out.WriteByte('<')
		out.WriteString(tag)
	}
	for {
		k, v, more := z.TagAttr()
		v = bytes.TrimSpace(v)

		if k := *(*string)(unsafe.Pointer(&k)); allowedAttrs[k] {
			out.WriteString(" ")
			out.WriteString(k)
			out.WriteString("='")
			if !bytes.HasPrefix(v, []byte("javascript")) {
				out.Write(v)
			}
			out.WriteString("'")
		}
		if !more {
			break
		}
	}
	out.WriteByte('>')
}

func RenderClip(v string) string {
	out := &escaper{}
	z := html.NewTokenizer(strings.NewReader(v))
	inCode := false

	var tagStack []string

	// out.WriteString("<div style='word-break:break-all'>")
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}

		tag, _ := z.TagName()
		tagStr := string(tag)

		switch tt {
		case html.StartTagToken:
			if tagStr == "eat" && !inCode {
				for out.Len() > 0 && unicode.IsSpace(rune(out.Bytes()[out.Len()-1])) {
					out.Truncate(out.Len() - 1)
				}
			} else if allowedTags[tagStr] && !inCode {
				out.writeStart(z, tagStr)
				tagStack = append(tagStack, tagStr)
				if tagStr == "code" {
					inCode = true
				}
			} else {
				out.writeEscape(z.Raw())
			}
		case html.SelfClosingTagToken:
			if allowedTags[tagStr] && !inCode {
				out.writeStart(z, tagStr)
				out.Truncate(out.Len() - 1)
				out.WriteString("/>")
			} else {
				out.writeEscape(z.Raw())
			}
		case html.EndTagToken:
			matched := false
			if tagStr == "code" || (allowedTags[tagStr] && !inCode) {
				if tagStr == "code" {
					inCode = false
				}

				for len(tagStack) > 0 {
					last := tagStack[len(tagStack)-1]
					tagStack = tagStack[:len(tagStack)-1]

					out.writeEnd(last)

					if last == tagStr {
						matched = true
						break
					}
				}
			}
			if !matched {
				out.writeEscape(z.Raw())
			}
		case html.DoctypeToken:
		default:
			out.writeEscape(z.Raw())
		}
	}

	for len(tagStack) > 0 {
		out.writeEnd(tagStack[len(tagStack)-1])
		tagStack = tagStack[:len(tagStack)-1]
	}
	// out.WriteString("</div>")

	return out.String()
}
