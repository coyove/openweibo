package types

import (
	"bytes"
	"strings"
	"unicode"
	"unsafe"

	"golang.org/x/net/html"
)

type escaper struct{ bytes.Buffer }

func (e *escaper) writeRaw(p string) {
	o := &e.Buffer
	for len(p) > 0 {
		idx := strings.IndexByte(p, '<')
		if idx == -1 {
			o.WriteString(p)
			break
		}
		o.WriteString(p[:idx])
		o.WriteString("&lt;")
		p = p[idx+1:]
	}
	return
}

func (e *escaper) writeText(p []byte) {
	o := &e.Buffer
	for len(p) > 0 {
		idx := bytes.IndexByte(p, '<')
		if idx == -1 {
			o.Write(p)
			break
		}
		o.Write(p[:idx])
		o.WriteString("&lt;")
		p = p[idx+1:]
	}
	return
}

var allowedAttrs = map[string]bool{
	"id": true, "name": true, "for": true, "style": true,
	"title": true, "src": true, "alt": true, "class": true,
	"href": true, "width": true, "height": true, "target": true,
}

var hideTags = map[string]bool{
	"script": true, "iframe": true,
}

var allowedTags = func() map[string]bool {
	m := map[string]bool{}
	for _, p := range strings.Split(`font,cite,kbd,meta,link,caption,small,style,
    abbr,b,em,pre,a,img,div,p,span,u,i,hr,br,strong,blockquote,video,code,ol,ul,li,
    ins,del,table,tr,tbody,thead,tfoot,td,th,h1,h2,h3,h4,label,font,textarea,input,
    sup,sub,dd,dl,dt,section,tt`, ",") {
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
		out.WriteString("<div style='position:relative;'><div style='white-space:pre;overflow-x:auto;overflow-y:hidden'><code")
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
				out.WriteString(html.EscapeString(*(*string)(unsafe.Pointer(&v))))
			}
			out.WriteString("'")
		}
		if !more {
			break
		}
	}
	out.WriteByte('>')
}

func TruncClip(v string, max int) string {
	z := html.NewTokenizer(strings.NewReader(v))
	out := &bytes.Buffer{}
	for out.Len() < max*3 {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.TextToken {
			out.Write(z.Raw())
		}
	}
	return UTF16Trunc(out.String(), max)
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
			if inCode {
				out.writeText(z.Raw())
			} else if tagStr == "eat" {
				if out.Len() == 0 {
					out.WriteString(`<style>.note{white-space:normal}</style>`)
				} else {
					for out.Len() > 0 && unicode.IsSpace(rune(out.Bytes()[out.Len()-1])) {
						out.Truncate(out.Len() - 1)
					}
				}
			} else if tagStr == "code" {
				out.writeStart(z, "code")
				inCode = true
			} else if allowedTags[tagStr] {
				out.writeStart(z, tagStr)
				tagStack = append(tagStack, tagStr)
			} else if !hideTags[tagStr] {
				out.writeText(z.Raw())
			}
		case html.SelfClosingTagToken:
			if inCode {
				out.writeText(z.Raw())
			} else if allowedTags[tagStr] {
				out.writeStart(z, tagStr)
				out.Truncate(out.Len() - 1)
				out.WriteString("/>")
			} else {
				out.writeText(z.Raw())
			}
		case html.EndTagToken:
			if tagStr == "code" {
				inCode = false
				out.writeEnd("code")
			} else if inCode {
				out.writeText(z.Raw())
			} else if allowedTags[tagStr] {
				for len(tagStack) > 0 {
					last := tagStack[len(tagStack)-1]
					tagStack = tagStack[:len(tagStack)-1]
					out.writeEnd(last)
					if last == tagStr {
						break
					}
				}
			} else if !hideTags[tagStr] {
				out.writeText(z.Raw())
			}
		case html.DoctypeToken:
		case html.CommentToken:
		default:
			out.writeText(z.Raw())
		}
	}

	for len(tagStack) > 0 {
		out.writeEnd(tagStack[len(tagStack)-1])
		tagStack = tagStack[:len(tagStack)-1]
	}
	// out.WriteString("</div>")

	return out.String()
}
