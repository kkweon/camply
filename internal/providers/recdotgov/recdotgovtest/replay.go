// Package recdotgovtest replays the recorded recreation.gov responses kept in
// internal/providers/recdotgov/testdata/input.
//
// It exists so that every test which needs a realistic Availability gets it the
// same way the binary does — from the provider, fed the API's own bytes — rather
// than by writing a core.Availability literal. A literal is free to describe a
// campground the adapter cannot actually produce, and one did: the notification
// golden asserted a recreation.gov site carrying the recreation area "Lake
// Tahoe" for months, while the adapter has never set that field. Tests built on
// this package cannot make that mistake, because they never name the field.
package recdotgovtest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Campgrounds maps a recreation.gov campground id to the directory holding its
// recorded metadata and availability.
//
// Several scenarios share one campground, so this is many-to-one by design: the
// recording exists once, and refreshing it updates every test built on it at the
// same time.
var Campgrounds = map[string]string{
	"10300216": "zephyr",
	"232461":   "lodgepole",
	"232875":   "kaspian",
	"10220612": "meeksbay",
}

// InputRoot is the recorded-response corpus, resolved relative to this file so
// callers work from any package's directory without knowing where they sit.
func InputRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("recdotgovtest: cannot resolve the input corpus path")
	}
	return filepath.Join(filepath.Dir(file), "..", "testdata", "input")
}

// NewServer starts a server replaying the recorded responses.
//
// Routing mirrors the three endpoints the provider calls: the campsite search
// (metadata, keyed by asset_id), the month availability (keyed by campground in
// the path), and the RIDB facility record that names the campground's recreation
// area and town. An unrecorded campground is a 404 rather than an empty body, so
// a test that asks for one fails loudly instead of silently finding nothing.
func NewServer(t *testing.T) *httptest.Server {
	t.Helper()

	root := InputRoot()
	serve := func(w http.ResponseWriter, campground, file string) {
		dir, ok := Campgrounds[campground]
		if !ok {
			http.Error(w, "no recorded input for campground "+campground, http.StatusNotFound)
			return
		}
		data, err := os.ReadFile(filepath.Join(root, dir, file))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/search/campsites":
			// fq=asset_id:232461
			fq := r.URL.Query().Get("fq")
			serve(w, strings.TrimPrefix(fq, "asset_id:"), "metadata.json")
		case strings.HasPrefix(r.URL.Path, "/api/camps/availability/campground/"):
			rest := strings.TrimPrefix(r.URL.Path, "/api/camps/availability/campground/")
			serve(w, strings.TrimSuffix(rest, "/month"), "availability.json")
		// Only the per-campground form. Bare /facilities is RIDB's nationwide
		// list, which is a different request with a different shape, and
		// answering it from one campground's record would be a lie.
		case strings.HasPrefix(r.URL.Path, "/facilities/"):
			serve(w, strings.TrimPrefix(r.URL.Path, "/facilities/"), "facility.json")
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}
