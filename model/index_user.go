package model

import (
	"time"
)

var userIndexQueue = make(chan *User, 1024)
var tagIndexQueue = make(chan string, 1024)

func init() {
	go func() {
		m := map[string]bool{}
		for start := time.Now(); ; {
			if time.Since(start).Seconds() > 5 {
				m = map[string]bool{}
				start = time.Now()
			}

			select {
			case u := <-userIndexQueue:
				if _, ok := m[u.ID]; ok {
					continue
				}
				Index("su", u.ID, u.ID+u.CustomName)
				m[u.ID] = true
			}
		}
	}()

	go func() {
		m := map[string]bool{}
		for start := time.Now(); ; {
			if time.Since(start).Seconds() > 5 {
				m = map[string]bool{}
				start = time.Now()
			}

			select {
			case t := <-tagIndexQueue:
				if _, ok := m[t]; ok {
					continue
				}
				Index("st", t, t)
				m[t] = true
			}
		}
	}()
}

func IndexUser(u *User, immediate bool) {
	if immediate {
		Index("su", u.ID, u.ID+u.CustomName)
	} else {
		select {
		case userIndexQueue <- u:
		default:
		}
	}
}

func IndexTag(t string) {
	select {
	case tagIndexQueue <- t:
	default:
	}
}
