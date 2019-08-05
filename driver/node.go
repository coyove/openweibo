package driver

import (
	"fmt"
	"time"
)

type Node struct {
	Name   string
	Weight int64
	KV
}

func (n *Node) String() string {
	return fmt.Sprintf("%s(w:%d,o:%d)", n.Name, n.Weight, n.Stat().ObjectCount)
}

type KV interface {
	Put(k string, v []byte) error
	Get(k string) ([]byte, error)
	Delete(k string) error
	Stat() Stat
}

type Stat struct {
	TotalBytes     int64
	AvailableBytes int64
	DownloadBytes  int64
	UploadBytes    int64
	Ping           int64
	ObjectCount    int64
	UpdateTime     time.Time
	Sealed         bool
	Error          error
	Throt          string
}

type StorageConfig struct {
	Name        string `yaml:"Name"`
	Type        string `yaml:"Type"`
	Weight      int64  `yaml:"Weight"`
	Throt       string `yaml:"Throt"`
	AccessToken string `yaml:"AccessToken"`
}
