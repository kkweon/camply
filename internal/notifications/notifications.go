package notifications

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kkweon/camply/internal/config"
	"github.com/kkweon/camply/internal/core"
)

// Notifier defines the interface for all notification providers
type Notifier interface {
	SendCampsites(campsites []core.AvailableCampsite) error
}

func getExampleCampsite() core.AvailableCampsite {
	return core.AvailableCampsite{
		CampsiteID:       "100",
		BookingDate:      time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC),
		BookingEndDate:   time.Date(2023, 9, 2, 0, 0, 0, 0, time.UTC),
		BookingNights:    1,
		CampsiteSiteName: "Test Campsite Name",
		CampsiteLoopName: "A1",
		CampsiteType:     "Test",
		// Deliberately a walk-in example: test-notifications is what proves the
		// warning path renders, and a drive-in sample would exercise the one
		// case that needs no warning.
		SiteAccess:         core.SiteAccessWalkIn,
		SiteAccessRaw:      "Walk-In",
		MaxVehicles:        new(int),
		MinOccupancy:       1,
		MaxOccupancy:       5,
		CampsiteUseType:    "Test",
		AvailabilityStatus: "Available",
		RecreationArea:     "Test Recreation Area",
		RecreationAreaID:   "20",
		FacilityName:       "Test Campground",
		FacilityID:         "50",
		BookingURL:         "https://youtu.be/eBGIQ7ZuuiU", // Keep the easter egg
	}
}

// formatMessage creates a standard HTML-formatted string for the campsite
func formatMessage(c core.AvailableCampsite) (string, string) {
	title := fmt.Sprintf("%s | %s | %s", c.RecreationArea, c.FacilityName, c.BookingDate.Format("2006-01-02"))
	// The title leads, and on a phone it is often all that is read before the
	// booking link is tapped. Everything about this site that needs checking
	// says so there, not only in the body.
	if prefix := c.WarningPrefix(); prefix != "" {
		title = prefix + " | " + title
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "<b>Campsite ID:</b> %s\n", c.CampsiteID)
	fmt.Fprintf(&buf, "<b>Booking Date:</b> %s\n", c.BookingDate.Format("2006-01-02"))
	fmt.Fprintf(&buf, "<b>Booking End Date:</b> %s\n", c.BookingEndDate.Format("2006-01-02"))
	fmt.Fprintf(&buf, "<b>Booking Nights:</b> %d\n", c.BookingNights)
	fmt.Fprintf(&buf, "<b>Campsite Site Name:</b> %s\n", c.CampsiteSiteName)
	fmt.Fprintf(&buf, "<b>Campsite Loop Name:</b> %s\n", c.CampsiteLoopName)
	fmt.Fprintf(&buf, "<b>Campsite Type:</b> %s\n", c.CampsiteType)
	// Unconditional, including the unknown case. Campsite Type alone caused the
	// incident: Zephyr Cove's hike-in sites are typed TENT ONLY NONELECTRIC,
	// exactly like its drive-in tent sites.
	fmt.Fprintf(&buf, "<b>Site Access:</b> %s\n", c.SiteAccessSummary())
	// Unconditional for the same reason as Site Access: an equipment filter can
	// let a site through without ever matching it, and the body is where that
	// has to be admitted.
	fmt.Fprintf(&buf, "<b>Equipment:</b> %s\n", c.EquipmentSummary())
	fmt.Fprintf(&buf, "<b>Campsite Occupancy:</b> %d-%d\n", c.MinOccupancy, c.MaxOccupancy)
	fmt.Fprintf(&buf, "<b>Campsite Use Type:</b> %s\n", c.CampsiteUseType)
	fmt.Fprintf(&buf, "<b>Availability Status:</b> %s\n", c.AvailabilityStatus)
	fmt.Fprintf(&buf, "<b>Recreation Area:</b> %s\n", c.RecreationArea)
	fmt.Fprintf(&buf, "<b>Recreation Area Id:</b> %s\n", c.RecreationAreaID)
	fmt.Fprintf(&buf, "<b>Facility Name:</b> %s\n", c.FacilityName)
	fmt.Fprintf(&buf, "<b>Facility Id:</b> %s\n", c.FacilityID)
	fmt.Fprintf(&buf, "<b>Booking Link:</b> <a href='%s'>%s</a>\n", c.BookingURL, c.BookingURL)

	return title, buf.String()
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

func (p *pushoverNotifier) SendCampsites(campsites []core.AvailableCampsite) error {
	for _, c := range campsites {
		title, message := formatMessage(c)

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

func (t *telegramNotifier) SendCampsites(campsites []core.AvailableCampsite) error {
	for _, c := range campsites {
		// Telegram doesn't use the title variable like Pushover, it just gets appended to the body
		_, message := formatMessage(c)

		payload := map[string]interface{}{
			"chat_id":    t.config.TelegramChatID,
			"text":       message,
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
		if err := n.SendCampsites([]core.AvailableCampsite{exampleSite}); err != nil {
			return fmt.Errorf("❌ Failed to send notification via %s: %w", providers[i], err)
		}
	}

	fmt.Println("✅ Notification test complete")
	return nil
}
