package main

import (
	"bytes"
	"path/filepath"
	"strconv"

	"github.com/sirupsen/logrus"
)

type LogFormatter struct{}

func (f *LogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	buf := bytes.Buffer{}
	if entry.Level <= logrus.ErrorLevel {
		buf.WriteString("ERR")
	} else {
		buf.WriteString("INFO")
	}
	buf.WriteString("\t")
	buf.WriteString(entry.Time.UTC().Format("2006-01-02T15:04:05.000\t"))
	if entry.Caller == nil {
		buf.WriteString("internal")
	} else {
		buf.WriteString(filepath.Base(entry.Caller.File))
		buf.WriteString(":")
		buf.WriteString(strconv.Itoa(entry.Caller.Line))
	}
	buf.WriteString("\t")
	buf.WriteString(entry.Message)
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}
