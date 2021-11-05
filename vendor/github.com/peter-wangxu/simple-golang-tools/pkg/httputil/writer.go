package httputil

import (
	"bytes"
	"net/http"
)

type WrappedResponseWriter struct {
	http.ResponseWriter
	buf      *bytes.Buffer
	httpCode *int
}

// TODO(peter) what if there are data/state already in the ResponseWriter?

func NewWrappedResponseWriter(w http.ResponseWriter) *WrappedResponseWriter {
	wrw := &WrappedResponseWriter{
		ResponseWriter: w,
		buf:            &bytes.Buffer{},
		httpCode:       new(int),
	}
	*wrw.httpCode = 200
	////// Try to read the existing data
	//if c, ok := w.(*WrappedResponseWriter); ok {
	//	wrw.buf.Write(c.Get())
	//	wrw.httpCode = c.httpCode
	//}
	return wrw
}

func (wrw *WrappedResponseWriter) Write(p []byte) (int, error) {
	n, err := wrw.ResponseWriter.Write(p)
	if err != nil { // if error, do not write buffer
		return n, err
	}
	return wrw.buf.Write(p)
}

func (wrw *WrappedResponseWriter) WriteHeader(code int) {
	*wrw.httpCode = code
	wrw.ResponseWriter.WriteHeader(code)
}

// Get returns all the written bytes, this make it
// possible to chain WrappedResponseWriter otherwise
// we lose the bytes written already
func (wrw *WrappedResponseWriter) Get() []byte {
	return wrw.buf.Bytes()
}

func (wrw *WrappedResponseWriter) Code() int {
	return *wrw.httpCode
}
