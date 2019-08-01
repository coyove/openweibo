package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	"github.com/coyove/ch"
	"github.com/coyove/ch/driver"
	"github.com/coyove/ch/driver/chdropbox"
	"gopkg.in/yaml.v2"
)

func main() {
	buf, err := ioutil.ReadFile("config.yml")
	if err != nil {
		panic(err)
	}

	config := map[interface{}]interface{}{}
	if err := yaml.Unmarshal(buf, config); err != nil {
		panic(err)
	}

	nodes := []*driver.Node{}
	storages, _ := config["Storages"].([]interface{})
	for _, s := range storages {
		s := s.(map[interface{}]interface{})
		name, ty := driver.Itos(s["Name"], ""), driver.Itos(s["Type"], "")
		if name == "" {
			panic("empty storage node name")
		}
		switch strings.ToLower(ty) {
		case "dropbox":
			log.Println("[config] load storage:", name)
			nodes = append(nodes, chdropbox.NewNode(name, s))
		default:
			panic("unknown storage type: " + ty)
		}
	}

	mgr := ch.Nodes{}
	mgr.LoadNodes(nodes)
	mgr.StartTransferAgent("transfer.db")
	fmt.Println(mgr.Put("zzz", []byte("hellow")))
	fmt.Println(mgr.Get("zzz"))
	fmt.Println(mgr.Get("zzz2"))
	fmt.Println(mgr.Delete("zzz"))
	fmt.Println(mgr.Get("zzz"))
	fmt.Println(nodes[0].Stat())
}
