package common

import (
	"github.com/coyove/iis/common/mail"
	"testing"
)

func TestFirstImage(t *testing.T) {
	MustLoadConfig("../config.json")
	t.Log(mail.SendMail("coyove@hotmail.com", "Password Reset", "New Password: 13"))
}
