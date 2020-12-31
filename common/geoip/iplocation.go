package geoip

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ip2location/ip2location-go"
)

var iplocdb *ip2location.DB

func open(path string) (*ip2location.DB, error) {
	return ip2location.OpenDB(path)
}

func LoadIPLocation() {
	fn, err := os.OpenFile(filepath.Join(os.TempDir(), "iploc"), os.O_CREATE|os.O_RDWR, 0777)
	if err != nil {
		panic(err)
	}

	if fi, _ := fn.Stat(); fi.Size() > 0 {
		fmt.Println("IPLocation DB is presented")
		fn.Close()

		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("%v", r)
				}
			}()
			iplocdb, err = open(fn.Name())
		}()

		if err != nil {
			fmt.Println("IPLocation DB error:", err, os.Remove(fn.Name()))
			LoadIPLocation()
		}
		return
	}

	req, _ := http.NewRequest("GET", "https://fweibomedia.s3.us-west-002.backblazeb2.com/IP2LOCATION-LITE-DB3.txt", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()

	fmt.Println("Downloading IPLocation DB")
	n, _ := io.Copy(fn, resp.Body)
	fmt.Println("Downloaded IPLocation DB:", n)

	fn.Close()

	iplocdb, err = open(fn.Name())
	if err != nil {
		panic(err)
	}
}

func LookupIP(ip string) (long, short string) {
	res, err := iplocdb.Get_all(ip)
	if err != nil {
		return
	}
	c := res.City
	if c == "-" {
		c = res.Region
		if c == "-" {
			return
		}
	}
	return res.Country_long + "-" + c, c
}
