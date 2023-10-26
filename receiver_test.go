package main

import (
	"net/http/httptest"
	"testing"
)

func TestReceiver(t *testing.T) {
	t.Run("test sse", func(t *testing.T) {
		res := httptest.NewRecorder()

		receiver := NewReceiverSSE(res, res)
		receiver.SendHostDisconnectedMessage()

		t.Log(res.Body.String())
	})
}
