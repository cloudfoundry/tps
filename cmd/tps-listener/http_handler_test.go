package main_test

import (
	"mime/multipart"
	"net/http"
	"regexp"
)

var processGuidPattern = regexp.MustCompile(`^/apps/([a-zA-Z0-9_-]+)/containermetrics$`)

type httpHandler struct {
	messages map[string][][]byte
}

func NewHttpHandler(m map[string][][]byte) *httpHandler {
	return &httpHandler{messages: m}
}

func (h *httpHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	mp := multipart.NewWriter(rw)
	defer mp.Close()

	guid := processGuidPattern.FindStringSubmatch(r.URL.Path)[1]

	rw.Header().Set("Content-Type", `multipart/x-protobuf; boundary=`+mp.Boundary())

	for _, msg := range h.messages[guid] {
		partWriter, _ := mp.CreatePart(nil)
		partWriter.Write(msg)
	}
}
