package recdotgov

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/kkweon/camply/internal/core"
)

func TestProvider_FindCampsites(t *testing.T) {
	// Arrange: Create a mock HTTP server to return our testdata
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search/campsites" {
			data, _ := os.ReadFile("testdata/metadata_response.json")
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}
		if r.URL.Path == "/api/camps/availability/campground/232447/month" {
			data, _ := os.ReadFile("testdata/availability_response.json")
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Inject the mock URL directly into the provider (we'll modify the provider to support this)
	p := &Provider{
		client:      server.Client(),
		apiScheme:   "http",
		apiNetLoc:   server.Listener.Addr().String(),
		apiBasePath: "api/camps/availability/campground",
	}

	req := core.SearchRequest{
		StartDates:  []time.Time{time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
		EndDates:    []time.Time{time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)},
		Campgrounds: []string{"232447"},
		Nights:      1,
	}

	// Act
	results, err := p.FindCampsites(context.Background(), req)
	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// In the mock data, there are 2 campsites total, but only 1 is available (for 2 consecutive nights)
	// Since we requested 1 night, it should return 2 AvailableCampsite records (June 1 and June 2).
	if len(results) != 2 {
		t.Errorf("expected 2 available campsite records, got %d", len(results))
	}
	}
