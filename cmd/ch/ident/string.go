package ident

func ParseID(s string) (id ID) {
	p, _ := idEncoding.DecodeString(s)
	id.Unmarshal(p)
	return
}

func (id ID) String() string {
	if !id.Valid() {
		return ""
	}
	buf := make([]byte, 20)
	id.marshal(buf[5:])
	idEncoding.Encode(buf, buf[5:])
	return string(buf)
}
