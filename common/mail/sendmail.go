package mail

import (
	"crypto/tls"
	"fmt"
	"github.com/coyove/iis/common"
	"net/smtp"
)

type loginAuth struct {
	username, password string
}

// func LoginAuth(username, password string) smtp.Auth {
// 	return &loginAuth{username, password}
// }

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, fmt.Errorf("unknown command: %q", fromServer)
		}
	}
	return nil, nil
}

func SendMail(to, subject, body string) error {
	server, me, password := common.Cfg.SMTPServer, common.Cfg.SMTPEmail, common.Cfg.SMTPPassword
	c, err := smtp.Dial(server)
	if err != nil {
		return err
	}

	if err := c.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
		return err
	}

	if err := c.Auth(&loginAuth{me, password}); err != nil {
		return err
	}

	if err := c.Mail(me); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}

	wc, err := c.Data()
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(wc, ("To: " + to + "\r\n" + "Subject: " + subject + "\r\n\r\n" + body + "\r\n")); err != nil {
		return err
	}

	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}
