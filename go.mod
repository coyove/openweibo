module github.com/coyove/iis

go 1.16

require (
	github.com/NYTimes/gziphandler v1.1.1
	github.com/aws/aws-sdk-go v1.28.2
	github.com/coyove/sdss v1.0.0
	github.com/gogo/protobuf v1.3.2
	github.com/pierrec/lz4/v4 v4.1.17
	github.com/sirupsen/logrus v1.8.1
	github.com/tidwall/gjson v1.14.4
	go.etcd.io/bbolt v1.3.6
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
	golang.org/x/net v0.0.0-20220722155237-a158d28d115b
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
)

replace github.com/coyove/sdss v1.0.0 => ../sdss

replace go.etcd.io/bbolt v1.3.6 => ./bbolt
