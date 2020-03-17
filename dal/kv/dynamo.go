package kv

import (
	"bytes"
	"log"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/coyove/iis/dal/kv/cache"
	//sync "github.com/sasha-s/go-deadlock"
)

var (
	dyTable  = "iis"
	dyTable2 = "iis2"
)

type DynamoKV struct {
	cache *cache.GlobalCache
	db    *dynamodb.DynamoDB
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
		db: db,
	}
	return r
}

func (m *DynamoKV) SetGlobalCache(c *cache.GlobalCache) {
	m.cache = c
}

func (m *DynamoKV) Get(key string) ([]byte, error) {
	nocache := false

	v, ok := m.cache.Get(key)
	if bytes.Equal(v, locker) {
		v = nil
		nocache = true
		// continue fetching value from dynamodb
	} else if ok {
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
	}

	if !nocache {
		if err := m.cache.Add(key, v); err != nil {
			log.Println("KV add:", err)
		}
	}

	return v, err
}

func (m *DynamoKV) Get2(key1, key2 string) ([]byte, error) {
	nocache := false

	v, ok := m.cache.Get(key1 + "..." + key2)
	if bytes.Equal(v, locker) {
		v = nil
		nocache = true
		// continue fetching value from dynamodb
	} else if ok {
		return v, nil
	}

	in := &dynamodb.GetItemInput{
		TableName: &dyTable2,
		Key: map[string]*dynamodb.AttributeValue{
			"id":  &dynamodb.AttributeValue{S: &key1},
			"id2": &dynamodb.AttributeValue{S: &key2},
		},
	}

	out, err := m.db.GetItem(in)
	if err != nil {
		return nil, err
	}

	if vi := out.Item["value"]; vi != nil && vi.S != nil {
		v = []byte(*vi.S)
	}

	if !nocache {
		if err := m.cache.Add(key1+"..."+key2, v); err != nil {
			log.Println("KV add:", err)
		}
	}

	return v, err
}

func (m *DynamoKV) Set(key string, value []byte) error {
	if err := m.cache.Add(key, locker); err != nil {
		return err
	}

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
	if err == nil {
		m.cache.Add(key, value)
	}
	return err
}

func (m *DynamoKV) Set2(key1, key2 string, value []byte) error {
	if err := m.cache.Add(key1+"..."+key2, locker); err != nil {
		return err
	}

	in := &dynamodb.UpdateItemInput{
		TableName: &dyTable2,
		Key: map[string]*dynamodb.AttributeValue{
			"id":  &dynamodb.AttributeValue{S: &key1},
			"id2": &dynamodb.AttributeValue{S: &key2},
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
	if err == nil {
		m.cache.Add(key1+"..."+key2, value)
	}
	return err
}

func (m *DynamoKV) Range(key, start string, n int) ([][]byte, string, error) {
	in := &dynamodb.QueryInput{
		TableName:              &dyTable2,
		KeyConditionExpression: aws.String("id = :id and id2 < :id2"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":id":  &dynamodb.AttributeValue{S: &key},
			":id2": &dynamodb.AttributeValue{S: &start},
		},
		Limit:            aws.Int64(int64(n)),
		ScanIndexForward: aws.Bool(false),
	}

	if start == "" {
		in.KeyConditionExpression = aws.String("id = :id")
	}

	out, err := m.db.Query(in)
	if err != nil {
		return nil, "", err
	}

	next := ""
	if out.LastEvaluatedKey != nil && out.LastEvaluatedKey["id2"] != nil {
		next = *out.LastEvaluatedKey["id2"].S
	}

	res := make([][]byte, len(out.Items))
	for i := range out.Items {
		res[i] = []byte(*out.Items[i]["value"].S)
	}
	return res, next, nil
}
