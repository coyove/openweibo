package middleware

import (
	"log"
	"regexp"
	"testing"
)

func TestPanic(t *testing.T) {
	rx := regexp.MustCompile(`((@|#)[^@# \n]+)`)
	log.Println(rx.FindAllStringSubmatch("#sfsdf\nwewed", -1))
}
