package providers

import (
	"context"

	"github.com/kkweon/camply/internal/core"
)

// Provider is the strict interface all campsite booking APIs must implement.
// No more leaky abstractions!
type Provider interface {
	// FindCampsites retrieves all raw availabilities from the API, already
	// mapped into camply's domain by the provider's adapter. No provider
	// vocabulary crosses this boundary.
	FindCampsites(ctx context.Context, req core.SearchRequest) ([]core.Availability, error)

	// FindCampgrounds searches for campgrounds
	FindCampgrounds(ctx context.Context, req core.SearchRequest) ([]core.CampgroundFacility, error)

	// FindRecreationAreas searches for recreation areas
	FindRecreationAreas(ctx context.Context, req core.SearchRequest) ([]core.RecreationArea, error)
}
