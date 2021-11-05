package httputil

import (
	"bytes"
	"io/ioutil"
	"net/http"
)

type WrappedRequest struct {
	*http.Request
	Buf  *bytes.Buffer
}

func NewWrappedRequest(r *http.Request) (wr *WrappedRequest) {
	reqBodyBytes, _ := ioutil.ReadAll(r.Body)
	r.Body.Close() //  must close

	r.Body = ioutil.NopCloser(bytes.NewBuffer(reqBodyBytes))
	wrInternal :=  &WrappedRequest{r, &bytes.Buffer{}}
	wrInternal.Buf.Write(reqBodyBytes)
	return wrInternal
}


func (wr *WrappedRequest) GetRequestBytes() []byte {
	if wr.Buf == nil {
		return []byte{}
	}
	return wr.Buf.Bytes()
}


