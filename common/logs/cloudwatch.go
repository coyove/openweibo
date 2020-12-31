package logs

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
)

const batchLimit = 900 * 1024 // CW limit: 1M, we use 900KB for safety

func now() string {
	return time.Now().Format(time.ANSIC)
}

type entry struct {
	ts  int64
	msg string
}

type Logger struct {
	group, stream string
	nextSeqToken  string
	c             *cloudwatchlogs.CloudWatchLogs
	ch            struct {
		sync.Mutex
		chain []entry
	}
}

func New(region, accessKey, secretKey, group, stream string) *Logger {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
		HTTPClient: &http.Client{
			Timeout: time.Second * 5,
			Transport: &http.Transport{
				MaxConnsPerHost: 10,
			},
		},
	})
	if err != nil {
		panic(err)
	}

	l := &Logger{
		c:      cloudwatchlogs.New(sess),
		group:  group,
		stream: stream,
	}

	l.nextSeqToken, err = l.getNextSeqToken()
	if err != nil {
		if os.Getenv("CW") == "0" {
			return l
		}
		panic(err)
	}

	go l.worker()

	return l
}

func (l *Logger) worker() {
	for {
		time.Sleep(time.Second)

	NEXT:
		in := &cloudwatchlogs.PutLogEventsInput{
			LogGroupName:  &l.group,
			LogStreamName: &l.stream,
			SequenceToken: &l.nextSeqToken,
		}

		if l.nextSeqToken == "" {
			in.SequenceToken = nil
		}

		totalSize := 0

		l.ch.Lock()
		for i, e := range l.ch.chain {
			if totalSize += len(e.msg) + 26; totalSize > batchLimit {
				l.ch.chain = l.ch.chain[i:]
				break
			}

			in.LogEvents = append(in.LogEvents, &cloudwatchlogs.InputLogEvent{
				Timestamp: aws.Int64(e.ts),
				Message:   aws.String(e.msg),
			})
		}

		if totalSize <= batchLimit {
			l.ch.chain = l.ch.chain[:0]
		}

		l.ch.Unlock()

		if len(in.LogEvents) == 0 {
			continue
		}

	IMMEDIATE_RETRY:
		out, err := l.c.PutLogEvents(in)
		if err != nil {
			er, _ := err.(awserr.Error)
			if er != nil {
				switch er.Code() {
				case cloudwatchlogs.ErrCodeInvalidSequenceTokenException,
					cloudwatchlogs.ErrCodeDataAlreadyAcceptedException:

					l.nextSeqToken, _ = l.getNextSeqToken()
					if l.nextSeqToken != "" {
						fmt.Println(now(), "Cloudwatch, retry new token:", l.nextSeqToken)
						goto IMMEDIATE_RETRY
					}
				}
			}

			fmt.Println(now(), "Cloudwatch put fatal:", err)

			f, err := os.OpenFile("badcw.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0700)
			if err != nil {
				fmt.Println(now(), "Cloudwatch second pass fatal error:", err)
			} else {
				p := bytes.Buffer{}
				for _, e := range in.LogEvents {
					p.WriteString(*e.Message)
					p.WriteString("\n")
				}
				f.Write(p.Bytes())
				f.Close()
			}
			continue
		}

		l.nextSeqToken = *out.NextSequenceToken

		if len(l.ch.chain) > 0 {
			goto NEXT
		}

		// sleep
	}
}

func (l *Logger) getNextSeqToken() (string, error) {
	out, err := l.c.DescribeLogStreams(&cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        &l.group,
		LogStreamNamePrefix: &l.stream,
		Descending:          aws.Bool(true),
		Limit:               aws.Int64(1),
	})
	if err != nil {
		return "", err
	}
	if len(out.LogStreams) == 1 && out.LogStreams[0].UploadSequenceToken != nil {
		return *out.LogStreams[0].UploadSequenceToken, nil
	}
	return "", nil
}

func (l *Logger) Write(p []byte) (int, error) {
	n := len(p)
	p = bytes.TrimRight(p, "\r\n")

	l.ch.Lock()
	l.ch.chain = append(l.ch.chain, entry{
		ts:  time.Now().UnixNano() / 1e6,
		msg: string(p),
	})
	if len(l.ch.chain) > 10000 { // Cloudwatch batch limit
		l.ch.chain = l.ch.chain[1:]
	}
	l.ch.Unlock()

	return n, nil
}
