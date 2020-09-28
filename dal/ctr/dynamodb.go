package ctr

import (
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

type DynamoBack struct {
	table string
	db    *dynamodb.DynamoDB
}

func NewDynamoBack(table string, sess *session.Session) (*DynamoBack, error) {
	db := dynamodb.New(sess)
	if _, err := db.DescribeEndpoints(&dynamodb.DescribeEndpointsInput{}); err != nil {
		return nil, err
	}
	return &DynamoBack{
		table: table,
		db:    db,
	}, nil
}

func (m *DynamoBack) get(k int64) int64 {
	in := &dynamodb.GetItemInput{
		TableName: &m.table,
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				N: aws.String(strconv.FormatInt(k, 10)),
			},
		},
		ConsistentRead: aws.Bool(true),
	}

	out, err := m.db.GetItem(in)
	if err != nil {
		panic(err)
	}

	if vi := out.Item["value"]; vi != nil && vi.N != nil {
		v, _ := strconv.ParseInt(*vi.N, 10, 64)
		return v
	}
	return 0
}

func (m *DynamoBack) set(key int64, value int64, override bool) (int64, bool) {
	in := &dynamodb.PutItemInput{
		TableName: &m.table,
		Item: map[string]*dynamodb.AttributeValue{
			"id": {
				N: aws.String(strconv.FormatInt(key, 10)),
			},
			"value": {
				N: aws.String(strconv.FormatInt(value, 10)),
			},
		},
		ReturnValues: aws.String("ALL_OLD"),
	}

	if override {
		resp, err := m.db.PutItem(in)
		if err != nil {
			panic(err)
		}
		vi := resp.Attributes["value"]
		if vi != nil {
			v, _ := strconv.ParseInt(*vi.N, 10, 64)
			return v, true
		}
		return 0, true
	}

	in.ConditionExpression = aws.String("attribute_not_exists(id)")
	_, err := m.db.PutItem(in)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == dynamodb.ErrCodeConditionalCheckFailedException {
				return m.get(key), false
			}
		}
		panic(err)
	}
	return value, true
}

func (m *DynamoBack) Set(key int64, value int64) int64 {
	v, _ := m.set(key, value, true)
	return v
}

func (m *DynamoBack) Put(key int64, value int64) (int64, bool) {
	return m.set(key, value, false)
}
