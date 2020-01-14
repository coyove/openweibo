package ik

import (
	"bytes"
	"compress/flate"
	"encoding/ascii85"
	"io/ioutil"
)

func CombineIDs(payload []byte, ids ...ID) string {
	if len(payload) == 0 && len(ids) == 0 {
		return ""
	}

	p := bytes.Buffer{}
	fill := [32]byte{}

	for i := 0; i <= len(ids)*9/2; i += len(fill) {
		p.Write(fill[:])
	}

	i := p.Len()

	w, _ := flate.NewWriter(&p, -1)

	xlen := 0
	for _, id := range ids {
		x := id.Marshal(fill[:])
		w.Write(x)
		xlen += len(x)
	}

	if len(payload) > 0 {
		fill[0] = 0
		w.Write(fill[:1])
		w.Write(payload)
	}

	if xlen == 0 && len(payload) == 0 {
		return ""
	}

	w.Close()

	tmp := p.Bytes()
	tmp = tmp[:ascii85.Encode(tmp, tmp[i:])]

	for i := range tmp {
		switch tmp[i] {
		case '"':
			tmp[i] = 'x'
		case '\\':
			tmp[i] = 'y'
		}
	}

	return string(tmp)
}

func SplitIDs(str string) (ids []ID, payload []byte) {
	if str == "" {
		return
	}

	tmp := []byte(str)
	for i := range tmp {
		switch tmp[i] {
		case 'x':
			tmp[i] = '"'
		case 'y':
			tmp[i] = '\\'
		}
	}

	n, _, _ := ascii85.Decode(tmp, tmp, true)
	tmp = tmp[:n]

	r := flate.NewReader(bytes.NewReader(tmp))
	for {
		id := ReadID(r)
		if !id.Valid() {
			payload, _ = ioutil.ReadAll(r)
			break
		}
		ids = append(ids, id)
	}
	r.Close()
	return
}
