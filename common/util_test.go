package common

import (
	"testing"
)

func TestFirstImage(t *testing.T) {
	MustLoadConfig("../config.json")
	t.Log(SendMail("coyove@hotmail.com", "Password Reset", "New Password: 13"))
}
