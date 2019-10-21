package kv

import (
	"log"
	"net/http"
	sync "sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/coyove/common/lru"
	"github.com/coyove/iis/cmd/ch/ident"
	//sync "github.com/sasha-s/go-deadlock"
)

var dyTable = "iis"
var rootID = ident.NewTagID("").String()

type DynamoKV struct {
	cache *lru.Cache
	db    *dynamodb.DynamoDB
	locks [65536]sync.Mutex
}

func NewDynamoKV(region, accessKey, secretKey string) *DynamoKV {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
		HTTPClient: &http.Client{
			Timeout: time.Second,
			Transport: &http.Transport{
				MaxConnsPerHost: 200,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	db := dynamodb.New(sess)
	_, err = db.DescribeEndpoints(&dynamodb.DescribeEndpointsInput{})
	if err != nil {
		panic(err)
	}
	r := &DynamoKV{
		db:    db,
		cache: lru.NewCache(CacheSize),
	}
	return r
}

func (m *DynamoKV) Lock(key string) {
	lk := &m.locks[hashString(key)]
	lk.Lock()
}

func (m *DynamoKV) Unlock(key string) {
	lk := &m.locks[hashString(key)]
	lk.Unlock()
}

func (m *DynamoKV) Get(key string) ([]byte, error) {
	x, _ := m.cache.Get(key)
	v, _ := x.([]byte)

	if len(v) > 0 {
		return v, nil
	}

	in := &dynamodb.GetItemInput{
		TableName: &dyTable,
		Key: map[string]*dynamodb.AttributeValue{
			"id": &dynamodb.AttributeValue{
				S: &key,
			},
		},
	}

	out, err := m.db.GetItem(in)
	if err != nil {
		return nil, err
	}

	if vi := out.Item["value"]; vi != nil && vi.S != nil {
		v = []byte(*vi.S)
		if key == rootID {
			log.Println("Last [TOOR]:", *vi.S)
		}
	}

	if len(v) > 0 {
		m.cache.Add(key, v)
	}
	return v, err
}

func (m *DynamoKV) Set(key string, value []byte) error {
	m.cache.Remove(key)

	in := &dynamodb.UpdateItemInput{
		TableName: &dyTable,
		Key: map[string]*dynamodb.AttributeValue{
			"id": &dynamodb.AttributeValue{
				S: &key,
			},
		},
		UpdateExpression: aws.String("set #xyzvalue = :value"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":value": &dynamodb.AttributeValue{
				S: aws.String(string(value)),
			},
		},
		ExpressionAttributeNames: map[string]*string{
			"#xyzvalue": aws.String("value"),
		},
	}
	_, err := m.db.UpdateItem(in)
	return err
}

func (m *DynamoKV) Delete(key string) error {
	m.cache.Remove(key)

	in := &dynamodb.DeleteItemInput{
		TableName: &dyTable,
		Key: map[string]*dynamodb.AttributeValue{
			"id": &dynamodb.AttributeValue{
				S: &key,
			},
		},
	}
	_, err := m.db.DeleteItem(in)
	return err
}
