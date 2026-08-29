package usedirect

// ReserveCalifornia runs on TylerTech's UseDirect platform. These URLs lived in
// the CLI, repeated once per command; they belong next to the provider that
// needs them.
const (
	ReserveCaliforniaName = "ReserveCalifornia"

	reserveCaliforniaBaseURL       = "https://california-rdr.prod.cali.rd12.recreation-management.tylerapp.com"
	reserveCaliforniaCampgroundURL = "https://www.reservecalifornia.com"
)

// NewReserveCalifornia builds the ReserveCalifornia provider.
//
// The generic NewProvider stays exported: tests inject an httptest URL through
// it, and other UseDirect deployments would be configured the same way.
func NewReserveCalifornia() *Provider {
	return NewProvider(ReserveCaliforniaName, reserveCaliforniaBaseURL, reserveCaliforniaCampgroundURL)
}
