package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Benchmark atomic view increment in Go
func BenchmarkIncrementView(b *testing.B) {
	initRedis()
	body := []byte(`{"videoId":"test-video-123","userId":"test-user-456"}`)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/views/increment", bytes.NewReader(body))
			w := httptest.NewRecorder()
			handleIncrementView(w, req)
			if w.Code != http.StatusOK {
				b.Errorf("Unexpected status: %d", w.Code)
			}
		}
	})
}

// Benchmark reading cached video feed from memory
func BenchmarkGetFeed(b *testing.B) {
	initRedis()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			handleGetFeed(w, req)
			if w.Code != http.StatusOK {
				b.Errorf("Unexpected status: %d", w.Code)
			}
		}
	})
}
