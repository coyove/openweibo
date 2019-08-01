package driver

import (
	"log"
	"testing"
)

func TestBucket(t *testing.T) {
	exit := make(chan bool, 2)
	bk := NewTokenBucket("10x2/20")
	go func() {
		log.Println("finished 1", bk.Consume(20))
		exit <- true
	}()
	go func() {
		log.Println("finished 2", bk.Consume(20))
		exit <- true
	}()
	<-exit
	<-exit
}
