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
			_, _ = w.Write(data)
			return
		}
		if r.URL.Path == "/api/camps/availability/campground/232447/month" {
			data, _ := os.ReadFile("testdata/availability_response.json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
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

// TestProvider_FindCampgrounds_RecAreaScoping guards a bug where --rec-area was
// accepted but never applied: FindCampgrounds always hit the flat /facilities
// endpoint, which has no rec-area filter, so `campsites --rec-area <id>` silently
// resolved to every reservable campground in the country.
func TestProvider_FindCampgrounds_RecAreaScoping(t *testing.T) {
	const scopedBody = `{"METADATA":{"RESULTS":{"CURRENT_COUNT":1,"TOTAL_COUNT":1}},` +
		`"RECDATA":[{"FacilityID":"232461","FacilityName":"Lodgepole","FacilityTypeDescription":"Campground",` +
		`"Enabled":true,"Reservable":true,"ParentRecAreaID":"2931",` +
		`"RECAREA":[{"RecAreaID":"2931","RecAreaName":"Sequoia & Kings Canyon National Parks"}]}]}`

	const flatBody = `{"METADATA":{"RESULTS":{"CURRENT_COUNT":1,"TOTAL_COUNT":1}},` +
		`"RECDATA":[{"FacilityID":"999999","FacilityName":"Some Campground In Another State",` +
		`"FacilityTypeDescription":"Campground","Enabled":true,"Reservable":true,"ParentRecAreaID":"1"}]}`

	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == "/recareas/2931/facilities" {
			_, _ = w.Write([]byte(scopedBody))
			return
		}
		_, _ = w.Write([]byte(flatBody))
	}))
	defer server.Close()

	p := &Provider{client: server.Client(), ridbBaseURL: server.URL}

	// With a rec-area, the nested endpoint must be used.
	got, err := p.FindCampgrounds(context.Background(), core.SearchRequest{RecreationArea: "2931"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(got) != 1 || got[0].FacilityID != "232461" {
		t.Fatalf("expected only the rec-area's campground, got %+v", got)
	}
	if gotPaths[0] != "/recareas/2931/facilities" {
		t.Errorf("expected nested rec-area endpoint, got %q", gotPaths[0])
	}

	// Without a rec-area, the flat endpoint is still correct.
	gotPaths = nil
	if _, err := p.FindCampgrounds(context.Background(), core.SearchRequest{}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotPaths[0] != "/facilities" {
		t.Errorf("expected flat endpoint, got %q", gotPaths[0])
	}
}

// TestProvider_FindCampgrounds_NoInfiniteLoop covers a page that claims a total but
// returns no rows: start never advanced, so the pagination loop spun forever.
func TestProvider_FindCampgrounds_NoInfiniteLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"METADATA":{"RESULTS":{"CURRENT_COUNT":0,"TOTAL_COUNT":50}},"RECDATA":[]}`))
	}))
	defer server.Close()

	p := &Provider{client: server.Client(), ridbBaseURL: server.URL}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = p.FindCampgrounds(context.Background(), core.SearchRequest{})
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("FindCampgrounds did not terminate: pagination loop is stuck")
	}
}
