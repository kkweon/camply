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
			data, _ := os.ReadFile("testdata/places.json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		if strings.Contains(r.URL.Path, "search/grid") {
			data, _ := os.ReadFile("testdata/grid.json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		if strings.Contains(r.URL.Path, "search/details") {
			data, _ := os.ReadFile("testdata/details.json")
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
		if c.Site.BookingURL != expectedURL {
			t.Errorf("expected BookingURL %q, got %q", expectedURL, c.Site.BookingURL)
		}
		if c.Site.Facility.RecreationArea != "Pismo SB" {
			t.Errorf("expected RecreationArea %q, got %q", "Pismo SB", c.Site.Facility.RecreationArea)
		}
		if c.Site.Facility.RecreationAreaID != "691" {
			t.Errorf("expected RecreationAreaID %q, got %q", "691", c.Site.Facility.RecreationAreaID)
		}
		if c.Site.MinOccupancy != 2 || c.Site.MaxOccupancy != 6 {
			t.Errorf("expected occupancy 2-6, got %d-%d", c.Site.MinOccupancy, c.Site.MaxOccupancy)
		}
	}
}

// newTestProvider wires a provider at a mock server serving the testdata fixtures.
func newTestProvider(t *testing.T) (*Provider, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for path, file := range map[string]string{
			"search/filters": "testdata/filters.json",
			"fd/facilities":  "testdata/facilities.json",
			"fd/places":      "testdata/places.json",
			"search/grid":    "testdata/grid.json",
			"search/details": "testdata/details.json",
		} {
			if strings.Contains(r.URL.Path, path) {
				data, _ := os.ReadFile(file)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	p := NewProvider("ReserveCalifornia", server.URL, "https://www.reservecalifornia.com")
	p.client = server.Client()
	return p, server.Close
}

func testSearchRequest() core.SearchRequest {
	start, _ := time.Parse("2006-01-02", "2026-05-22")
	end, _ := time.Parse("2006-01-02", "2026-05-23")
	return core.SearchRequest{
		StartDates:  []time.Time{start},
		EndDates:    []time.Time{end},
		Campgrounds: []string{"616"},
		Nights:      1,
	}
}

// The grid endpoint ignores MinVehicleLength in the request — verified against
// the live API, which returns identical unit counts with the parameter set to
// anything or omitted — so the filter has to run against the VehicleLength each
// unit reports back.
func TestFindCampsites_MinVehicleLengthFiltersOnTheResponse(t *testing.T) {
	p, closeFn := newTestProvider(t)
	defer closeFn()

	unfiltered, err := p.FindCampsites(context.Background(), testSearchRequest())
	if err != nil {
		t.Fatalf("unfiltered search failed: %v", err)
	}
	if len(unfiltered) == 0 {
		t.Fatal("fixture produced no campsites, so the filter cannot be exercised")
	}

	p2, closeFn2 := newTestProvider(t)
	defer closeFn2()
	req := testSearchRequest()
	req.MinVehicleLength = 999 // longer than any fixture unit

	filtered, err := p2.FindCampsites(context.Background(), req)
	if err != nil {
		t.Fatalf("filtered search failed: %v", err)
	}
	if len(filtered) != 0 {
		t.Errorf("got %d campsites for a 999 ft requirement, want 0", len(filtered))
	}
}

// Every equipment name the provider emits must be a member of the declared
// vocabulary, or the validator built on it would drift from what searches return.
func TestEmittedEquipmentStaysWithinTheDeclaredVocabulary(t *testing.T) {
	p, closeFn := newTestProvider(t)
	defer closeFn()

	sites, err := p.FindCampsites(context.Background(), testSearchRequest())
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	known := map[string]bool{}
	for _, n := range SynthesizedEquipment {
		known[n] = true
	}
	for _, s := range sites {
		for _, e := range s.Site.Equipment {
			if !known[e.EquipmentName] {
				t.Errorf("campsite %s reports %q, which is not in SynthesizedEquipment",
					s.Site.ID, e.EquipmentName)
			}
		}
	}
}
