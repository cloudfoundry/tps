package main_test

import (
	"mime/multipart"
	"net/http"
)

type httpHandler struct {
	messages [][]byte
}

func NewHttpHandler(m [][]byte) *httpHandler {
	return &httpHandler{messages: m}
}

func (h *httpHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	mp := multipart.NewWriter(rw)
	defer mp.Close()

	rw.Header().Set("Content-Type", `multipart/x-protobuf; boundary=`+mp.Boundary())

	for _, msg := range h.messages {
		partWriter, _ := mp.CreatePart(nil)
		partWriter.Write(msg)
	}
}
