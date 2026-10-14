package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"signalpost/internal/model"
)

func TestFetchClientCachingAndHardCap(t *testing.T) {
	requestHits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestHits++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "hello"}`))
	}))
	defer ts.Close()

	tempDir, err := os.MkdirTemp("", "fetch_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	report := model.NewRunReport()
	client, err := NewClient(Options{
		MaxRequests: 2, // Hard cap of 2 requests
		Timeout:     5 * time.Second,
		CacheDir:    filepath.Join(tempDir, "cache"),
		Report:      report,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ctx := context.Background()

	// 1. First fetch -> cache miss, hits server
	resp1, err := client.Get(ctx, ts.URL+"/test", 1*time.Hour)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	if resp1.FromCache {
		t.Errorf("First request should not be from cache")
	}
	if client.RequestCount() != 1 {
		t.Errorf("RequestCount = %d; want 1", client.RequestCount())
	}
	if requestHits != 1 {
		t.Errorf("Server hits = %d; want 1", requestHits)
	}

	// 2. Second fetch to same URL -> cache hit, does not hit server or increment counter
	resp2, err := client.Get(ctx, ts.URL+"/test", 1*time.Hour)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}
	if !resp2.FromCache {
		t.Errorf("Second request should be from cache")
	}
	if client.RequestCount() != 1 {
		t.Errorf("RequestCount after cache hit = %d; want 1", client.RequestCount())
	}
	if requestHits != 1 {
		t.Errorf("Server hits after cache hit = %d; want 1", requestHits)
	}

	// 3. Second unique request -> hits server, counter becomes 2 (hits cap)
	_, err = client.Get(ctx, ts.URL+"/test2", 1*time.Hour)
	if err != nil {
		t.Fatalf("Second unique request failed: %v", err)
	}
	if client.RequestCount() != 2 {
		t.Errorf("RequestCount = %d; want 2", client.RequestCount())
	}

	// 4. Third unique request -> should fail with ErrHardCapExceeded
	_, err = client.Get(ctx, ts.URL+"/test3", 1*time.Hour)
	if !errors.Is(err, ErrHardCapExceeded) {
		t.Errorf("Expected ErrHardCapExceeded, got: %v", err)
	}
	if !client.HardCapReached() {
		t.Errorf("client.HardCapReached() should be true")
	}
}
