package id

func BytesString(p []byte) string {
	return idEncoding.EncodeToString(p)
}

func StringBytes(s string) []byte {
	b, _ := idEncoding.DecodeString(s)
	return b
}
