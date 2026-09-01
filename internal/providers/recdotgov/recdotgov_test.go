package recdotgov

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/providers/recdotgov/recdotgovtest"
)

// TestProvider_FindCampsites runs the provider against recreation.gov's own
// recorded bytes rather than a hand-written stand-in.
//
// The stand-in it replaces described a "Test Campground" with campsite ids 100
// and 101 and two equipment entries — a shape the live API never returns. A
// fixture that small cannot fail the way production fails, so it certified the
// adapter without exercising it.
func TestProvider_FindCampsites(t *testing.T) {
	srv := recdotgovtest.NewServer(t)
	p := NewProviderAt("http", srv.Listener.Addr().String())

	req := core.SearchRequest{
		StartDates:  []time.Time{time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)},
		EndDates:    []time.Time{time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)},
		Campgrounds: []string{"10300216"},
		Nights:      2,
	}

	results, err := p.FindCampsites(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) == 0 {
		t.Fatal("the recording holds availability; the provider found none")
	}

	for _, a := range results {
		if a.Site == nil {
			t.Fatal("availability with no site")
		}
		if a.Site.ID == "" || a.Site.Name == "" {
			t.Errorf("site is unidentifiable: id=%q name=%q", a.Site.ID, a.Site.Name)
		}
		// Nights is the length of the stay found, not the length asked for:
		// the request is a window the provider slides a stay through. What must
		// hold is that the dates and the count describe the same stay.
		if a.Nights < 1 {
			t.Errorf("campsite %s: stay of %d nights", a.Site.ID, a.Nights)
		}
		if want := a.Start.AddDate(0, 0, a.Nights); !a.End.Equal(want) {
			t.Errorf("campsite %s: %d nights from %s ends %s, want %s",
				a.Site.ID, a.Nights, a.Start.Format("2006-01-02"),
				a.End.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	}
}

// TestProvider_FindCampsites_IdentifiesTheCampground is the assertion the
// notification title depends on: a camper who does not memorise campground names
// needs to be told where the site is, and the provider is the only layer that
// knows.
//
// It reads the fields off replayed output on purpose. Asserting this against a
// literal would prove nothing, since a literal can simply declare the recreation
// area the adapter never sets.
func TestProvider_FindCampsites_IdentifiesTheCampground(t *testing.T) {
	srv := recdotgovtest.NewServer(t)
	p := NewProviderAt("http", srv.Listener.Addr().String())

	results, err := p.FindCampsites(context.Background(), core.SearchRequest{
		StartDates:  []time.Time{time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)},
		EndDates:    []time.Time{time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)},
		Campgrounds: []string{"10300216"},
		Nights:      2,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) == 0 {
		t.Fatal("the recording holds availability; the provider found none")
	}

	for _, a := range results {
		f := a.Site.Facility
		if f.ID == "" || f.Name == "" {
			t.Errorf("campground unidentified: id=%q name=%q", f.ID, f.Name)
		}
		if f.RecreationArea == "" {
			t.Errorf("campsite %s reports no recreation area: a notification titled "+
				"%q tells the reader nothing about where the site is", a.Site.ID, f.Name)
		}
		if f.RecreationAreaID == "" {
			t.Errorf("campsite %s reports no recreation area id", a.Site.ID)
		}
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
