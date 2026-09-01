package notifications

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kkweon/camply/internal/config"
	"github.com/kkweon/camply/internal/core"
)

// Notifier defines the interface for all notification providers
type Notifier interface {
	SendCampsites(bookings []core.Availability) error
}

func getExampleCampsite() core.Availability {
	zero := 0
	return core.Availability{
		Site: &core.Site{
			ID:   "100",
			Name: "Test Campsite Name",
			Loop: "A1",
			Facility: core.Facility{
				ID:               "50",
				Name:             "Test Campground",
				RecreationArea:   "Test Recreation Area",
				RecreationAreaID: "20",
				Location:         "Test Town, CA",
			},
			// Deliberately a walk-in example: test-notifications is what proves
			// the warning path renders, and a drive-in sample would exercise the
			// one case that needs no warning.
			Parking:      core.ParkingWalk,
			AccessLabel:  "Walk-In",
			MaxVehicles:  &zero,
			RawType:      "Test",
			UseType:      "Test",
			MinOccupancy: 1,
			MaxOccupancy: 5,
			BookingURL:   "https://youtu.be/eBGIQ7ZuuiU", // Keep the easter egg
		},
		Start:  time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC),
		End:    time.Date(2023, 9, 2, 0, 0, 0, 0, time.UTC),
		Nights: 1,
		Status: "Available",
	}
}

// formatMessage creates the title and HTML body for one availability, ordered
// by what the reader decides with it: warnings first, then when and what the
// site is, then the booking URL, then the spec lines, debug ids last.
func formatMessage(a core.Availability) (string, string) {
	// Joined from the parts that exist, never formatted positionally: a
	// provider that reports no recreation area used to produce a title opening
	// on an empty field and a bare pipe.
	var parts []string
	for _, part := range []string{
		a.Site.Facility.RecreationArea,
		a.Site.Facility.Name,
		a.Start.Format("2006-01-02"),
	} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	title := strings.Join(parts, " | ")
	// The title leads, and on a phone it is often all that is read before the
	// booking link is tapped. Everything about this site that needs checking
	// says so there, not only in the body.
	if prefix := a.WarningPrefix(); prefix != "" {
		title = prefix + " | " + title
	}

	var sections []string
	if w := warningBlock(a); len(w) > 0 {
		sections = append(sections, strings.Join(w, "\n"))
	}
	sections = append(sections,
		strings.Join(summaryLines(a), "\n"),
		// A bare URL, not an anchor: notification shades strip HTML, and the
		// visible URL is what lets Android offer its open-link quick action.
		"👉 "+a.Site.BookingURL,
		// Equipment stays unconditional, unknown case included: an equipment
		// filter can let a site through without ever matching it, and the
		// body is where that has to be admitted.
		"<b>Equipment:</b> "+a.EquipmentSummary()+"\n<b>Hookups:</b> "+a.HookupsSummary(),
		fmt.Sprintf("<i>🔧 ids: campsite %s · facility %s · rec area %s</i>",
			a.Site.ID, a.Site.Facility.ID, a.Site.Facility.RecreationAreaID),
	)

	return title, strings.Join(sections, "\n\n") + "\n"
}

// warningBlock is every line that belongs above the fold: doubts the reader
// must resolve before tapping the booking URL below them.
func warningBlock(a core.Availability) []string {
	var lines []string
	// The same predicate decides warning-block versus inline on the site line,
	// so access appears exactly once for every input — the incident rule is
	// that it is never omitted, not that it always alarms.
	if a.SiteAccessAlert() != "" {
		lines = append(lines, a.SiteAccessSummary())
	}
	if a.EquipmentUnverified {
		lines = append(lines, "⚠️ NO EQUIPMENT DATA — verify on the booking page")
	}
	if u := strings.TrimSpace(a.Site.UseType); u != "" && !strings.EqualFold(u, "Overnight") {
		lines = append(lines, "⚠️ "+u)
	}
	return lines
}

// summaryLines is the where-when-and-what block: the town, the dates, the site
// itself, and what it takes — the facts a camper scans before deciding the URL
// is worth tapping.
func summaryLines(a core.Availability) []string {
	nights := "nights"
	if a.Nights == 1 {
		nights = "night"
	}
	var lines []string
	// Where, before when. The title names the campground, and a reader watching
	// a dozen of them does not carry a map of which town each sits in.
	if loc := a.Site.Facility.Location; loc != "" {
		lines = append(lines, "📍 "+loc)
	}
	lines = append(lines, fmt.Sprintf("📅 %s → %s · %d %s",
		a.Start.Format("2006-01-02"), a.End.Format("2006-01-02"), a.Nights, nights))

	site := "🏕️ Site " + a.Site.Name
	if a.Site.Loop != "" {
		site += " · Loop " + a.Site.Loop
	}
	site += fmt.Sprintf(" · %d–%d people", a.Site.MinOccupancy, a.Site.MaxOccupancy)
	if a.SiteAccessAlert() == "" {
		site += " · " + a.SiteAccessSummary()
	}
	lines = append(lines, site)

	// The provider's type plus the three axes it cannot express: a STANDARD
	// site takes a tent and an RV both, and its type says neither.
	lines = append(lines, "⛺ "+a.Site.RawType+" · accepts: "+a.PermitsSummary())
	return lines
}

type pushoverNotifier struct {
	client *http.Client
	config *config.AppConfig
	// token is the resolved Pushover application token: the user's own if set,
	// otherwise camply's shared default app.
	token string
}

// pushoverDefaultAPIToken is camply's own registered Pushover application, stored
// base64-encoded to match the Python implementation (camply/config/notification_config.py).
// Users only need to supply PUSHOVER_PUSH_USER; the app token is optional.
const pushoverDefaultAPIToken = "YXBqOWlzNjRrdm5zZWt3YmEyeDZxaDV0cWhxbXI5"

// NewPushover creates a new Pushover notifier if the config is valid.
// Only PUSHOVER_PUSH_USER is required: Pushover's user key identifies the
// recipient, while the token identifies the sending application, so an unset
// token falls back to camply's shared app rather than being an error.
func NewPushover(cfg *config.AppConfig) (Notifier, error) {
	if cfg.PushoverPushUser == "" {
		return nil, fmt.Errorf("pushover requires PUSHOVER_PUSH_USER in ~/.camply")
	}

	token := cfg.PushoverPushToken
	if token == "" {
		decoded, err := base64.StdEncoding.DecodeString(pushoverDefaultAPIToken)
		if err != nil {
			return nil, fmt.Errorf("failed to decode default Pushover token: %w", err)
		}
		token = string(decoded)
	}

	return &pushoverNotifier{
		client: &http.Client{Timeout: 5 * time.Second},
		config: cfg,
		token:  token,
	}, nil
}

func (p *pushoverNotifier) SendCampsites(bookings []core.Availability) error {
	for _, b := range bookings {
		title, message := formatMessage(b)

		payload := map[string]interface{}{
			"token":   p.token,
			"user":    p.config.PushoverPushUser,
			"message": message,
			"title":   title,
			"html":    1, // Enable HTML rendering
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		resp, err := p.client.Post("https://api.pushover.net/1/messages.json", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to send Pushover message: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("pushover API returned status: %d", resp.StatusCode)
		}
	}
	return nil
}

type telegramNotifier struct {
	client *http.Client
	config *config.AppConfig
}

// NewTelegram creates a new Telegram notifier if the config is valid
func NewTelegram(cfg *config.AppConfig) (Notifier, error) {
	if cfg.TelegramBotToken == "" || cfg.TelegramChatID == "" {
		return nil, fmt.Errorf("telegram requires both TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID in ~/.camply")
	}
	return &telegramNotifier{
		client: &http.Client{Timeout: 5 * time.Second},
		config: cfg,
	}, nil
}

func (t *telegramNotifier) SendCampsites(bookings []core.Availability) error {
	for _, b := range bookings {
		// Telegram has no separate title field, and the body alone no longer
		// carries the location — the title is where it lives now.
		title, message := formatMessage(b)

		payload := map[string]interface{}{
			"chat_id":    t.config.TelegramChatID,
			"text":       "<b>" + title + "</b>\n\n" + message,
			"parse_mode": "HTML",
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.config.TelegramBotToken)
		resp, err := t.client.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to send Telegram message: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("telegram API returned status: %d", resp.StatusCode)
		}
	}
	return nil
}

func SetupNotifiers(providers []string, cfg *config.AppConfig) ([]Notifier, error) {
	var notifiers []Notifier
	var errs []error

	for _, p := range providers {
		switch p {
		case "pushover":
			n, err := NewPushover(cfg)
			if err != nil {
				errs = append(errs, err)
			} else {
				notifiers = append(notifiers, n)
			}
		case "telegram":
			n, err := NewTelegram(cfg)
			if err != nil {
				errs = append(errs, err)
			} else {
				notifiers = append(notifiers, n)
			}
		default:
			fmt.Printf("⚠️  Warning: Notification provider '%s' is not yet implemented in the Go rewrite\n", p)
		}
	}

	if len(errs) > 0 {
		return notifiers, fmt.Errorf("encountered errors setting up notifiers: %v", errs)
	}

	return notifiers, nil
}

// RunTestNotifications executes the test flow just like the Python version
func RunTestNotifications(providers []string, cfg *config.AppConfig) error {
	notifiers, err := SetupNotifiers(providers, cfg)
	if err != nil {
		return err
	}

	exampleSite := getExampleCampsite()

	for i, n := range notifiers {
		fmt.Printf("Testing notification provider: %s\n", providers[i])
		if err := n.SendCampsites([]core.Availability{exampleSite}); err != nil {
			return fmt.Errorf("❌ Failed to send notification via %s: %w", providers[i], err)
		}
	}

	fmt.Println("✅ Notification test complete")
	return nil
}
