package common

import "unicode/utf8"

const CompressedLength = 15

func SafeStringForCompressString(id string) string {
	buf := make([]rune, 0, len(id))
	count := 0
	for _, c := range id {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '.', c == '-', c == '~', c == '!':
			buf = append(buf, c)
			count++
		case c >= 0x2000 && c <= 0x9fff:
			if count > CompressedLength-2 {
				return string(buf)
			}
			buf = append(buf, c)
			count += 2
		default:
			buf = append(buf, '_')
			count++
		}
		if count == CompressedLength {
			break
		}
	}
	return string(buf)
}

func CompressString(id string) []byte {
	buf := make([]byte, 0, len(id))
	for _, c := range id {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '.', c == '-', c == '~', c == '!':
			buf = append(buf, byte(c))
		case c >= 0x2000 && c <= 0x9fff:
			if len(buf) > CompressedLength-2 {
				return buf
			}
			c = (c - 0x2000) | 0x8000
			buf = append(buf, byte(c>>8), byte(c))
		default:
			buf = append(buf, '_')
		}
		if len(buf) == CompressedLength {
			break
		}
	}
	return buf
}

func DecompressString(buf []byte) string {
	p := make([]byte, 0, len(buf))
	for i := 0; i < len(buf); {
		c := buf[i]
		if c < 0x80 {
			p = append(p, c)
			i++
			continue
		}
		if i == len(buf)-1 {
			// invalid
			return ""
		}
		p = append(p, 0, 0, 0)
		utf8.EncodeRune(p[len(p)-3:], (rune(c)<<8+rune(buf[i+1]))&0x7fff+0x2000)
		i += 2
	}
	return string(p)
}
