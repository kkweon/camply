package usedirect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kkweon/camply/internal/core"
)

func TestProvider_FindCampsites_WithEquipmentAndURL(t *testing.T) {
	// Arrange: Create a mock HTTP server to return our testdata
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "search/filters") {
			data, _ := os.ReadFile("testdata/filters.json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		if strings.Contains(r.URL.Path, "fd/facilities") {
			data, _ := os.ReadFile("testdata/facilities.json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		if strings.Contains(r.URL.Path, "fd/places") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("[]")) // Mock empty places for warmup
			return
		}
		if strings.Contains(r.URL.Path, "search/grid") {
			data, _ := os.ReadFile("testdata/grid.json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Inject the mock URL directly into the provider
	p := NewProvider("ReserveCalifornia", server.URL, "https://www.reservecalifornia.com")
	p.client = server.Client() // use test client

	start, _ := time.Parse("2006-01-02", "2026-05-22")
	end, _ := time.Parse("2006-01-02", "2026-05-23")
	req := core.SearchRequest{
		StartDates:  []time.Time{start},
		EndDates:    []time.Time{end},
		Campgrounds: []string{"616"},
		Nights:      1,
		Equipment: []core.Equipment{
			{EquipmentName: "Tent", MaxLength: 0},
		},
	}

	// Act
	rawCampsites, err := p.FindCampsites(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Apply filter manually for test
	filter := core.Filter{}
	results := filter.Apply(rawCampsites, req)

	// Assert
	if len(results) != 34 { // We expect 34 records
		t.Errorf("expected 34 available tent campsite records, got %d", len(results))
	}

	for _, c := range results {
		expectedURL := "https://www.reservecalifornia.com/park/691/616"
		if c.BookingURL != expectedURL {
			t.Errorf("expected BookingURL %q, got %q", expectedURL, c.BookingURL)
		}
	}
}
