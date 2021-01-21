package ik

import (
	"bytes"
	"compress/flate"
	"io/ioutil"
)

func CombineIDs(payload []byte, ids ...ID) string {
	if len(payload) == 0 && len(ids) == 0 {
		return ""
	}

	p := &bytes.Buffer{}
	w, _ := flate.NewWriter(p, -1)

	fill, xlen := [32]byte{}, 0
	for _, id := range ids {
		x := id.Marshal(fill[:])
		xlen += len(x)
		w.Write(x)
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
	return idEncoding.EncodeToString(p.Bytes())
}

func SplitIDs(str string) (ids []ID, payload []byte) {
	tmp, _ := idEncoding.DecodeString(str)
	if len(tmp) == 0 {
		return
	}

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
