package limiter

import (
	"fmt"
	"io"
	"net/http"
)

type maxBytesReader struct {
	rdr        io.ReadCloser
	remaining  int64
	wasAborted bool
	sawEOF     bool
}

var ErrRequestTooLarge = fmt.Errorf("request too large")

func (mbr *maxBytesReader) tooLarge() (n int, err error) {
	n, err = 0, ErrRequestTooLarge

	if !mbr.wasAborted {
		mbr.wasAborted = true
	}
	return
}

func (mbr *maxBytesReader) Read(p []byte) (n int, err error) {
	toRead := mbr.remaining
	if mbr.remaining == 0 {
		if mbr.sawEOF {
			return mbr.tooLarge()
		}
		// The underlying io.Reader may not return (0, io.EOF)
		// at EOF if the requested size is 0, so read 1 byte
		// instead. The io.Reader docs are a bit ambiguous
		// about the return value of Read when 0 bytes are
		// requested, and {bytes,strings}.Reader gets it wrong
		// too (it returns (0, nil) even at EOF).
		toRead = 1
	}
	if int64(len(p)) > toRead {
		p = p[:toRead]
	}
	n, err = mbr.rdr.Read(p)
	if err == io.EOF {
		mbr.sawEOF = true
	}
	if mbr.remaining == 0 {
		// If we had zero bytes to read remaining (but hadn't seen EOF)
		// and we get a byte here, that means we went over our limit.
		if n > 0 {
			return mbr.tooLarge()
		}
		return 0, err
	}
	mbr.remaining -= int64(n)
	if mbr.remaining < 0 {
		mbr.remaining = 0
	}
	return
}

func (mbr *maxBytesReader) Close() error {
	return mbr.rdr.Close()
}

func LimitRequestSize(r *http.Request, size int64) *http.Request {
	r.Body = &maxBytesReader{
		rdr:       r.Body,
		remaining: size,
	}
	return r
}
