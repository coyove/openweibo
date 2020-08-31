package model

import "testing"

func BenchmarkMarhsal(b *testing.B) {
	v := "{\"ArticleID\":\"coyove\",\"Session\":\"\",\"Role\":\"\",\"PasswordHash\":\"\",\"e\":\"admin@fff\",\"av\":1,\"cn\":\"\xe7\xae\xa1\xe7\x90\x86\xe4\xba\xba\",\"F\":17,\"f\":37,\"ur\":0,\"sip\":\"\",\"st\":0,\"lt\":1582794331,\"kmc\":15}"
	for i := 0; i < b.N; i++ {
		UnmarshalUser([]byte(v))
	}
}
