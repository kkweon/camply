package notifications

import (
	"bytes"
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
		CampsiteID:         "100",
		BookingDate:        time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC),
		BookingEndDate:     time.Date(2023, 9, 2, 0, 0, 0, 0, time.UTC),
		BookingNights:      1,
		CampsiteSiteName:   "Test Campsite Name",
		CampsiteLoopName:   "A1",
		CampsiteType:       "Test",
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

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("<b>Campsite ID:</b> %s\n", c.CampsiteID))
	buf.WriteString(fmt.Sprintf("<b>Booking Date:</b> %s\n", c.BookingDate.Format("2006-01-02")))
	buf.WriteString(fmt.Sprintf("<b>Booking End Date:</b> %s\n", c.BookingEndDate.Format("2006-01-02")))
	buf.WriteString(fmt.Sprintf("<b>Booking Nights:</b> %d\n", c.BookingNights))
	buf.WriteString(fmt.Sprintf("<b>Campsite Site Name:</b> %s\n", c.CampsiteSiteName))
	buf.WriteString(fmt.Sprintf("<b>Campsite Loop Name:</b> %s\n", c.CampsiteLoopName))
	buf.WriteString(fmt.Sprintf("<b>Campsite Type:</b> %s\n", c.CampsiteType))
	buf.WriteString(fmt.Sprintf("<b>Campsite Occupancy:</b> %d-%d\n", c.MinOccupancy, c.MaxOccupancy))
	buf.WriteString(fmt.Sprintf("<b>Campsite Use Type:</b> %s\n", c.CampsiteUseType))
	buf.WriteString(fmt.Sprintf("<b>Availability Status:</b> %s\n", c.AvailabilityStatus))
	buf.WriteString(fmt.Sprintf("<b>Recreation Area:</b> %s\n", c.RecreationArea))
	buf.WriteString(fmt.Sprintf("<b>Recreation Area Id:</b> %s\n", c.RecreationAreaID))
	buf.WriteString(fmt.Sprintf("<b>Facility Name:</b> %s\n", c.FacilityName))
	buf.WriteString(fmt.Sprintf("<b>Facility Id:</b> %s\n", c.FacilityID))
	buf.WriteString(fmt.Sprintf("<b>Booking Link:</b> <a href='%s'>%s</a>\n", c.BookingURL, c.BookingURL))

	return title, buf.String()
}

type pushoverNotifier struct {
	client *http.Client
	config *config.AppConfig
}

// NewPushover creates a new Pushover notifier if the config is valid
func NewPushover(cfg *config.AppConfig) (Notifier, error) {
	if cfg.PushoverPushToken == "" || cfg.PushoverPushUser == "" {
		return nil, fmt.Errorf("Pushover requires both PUSHOVER_PUSH_TOKEN and PUSHOVER_PUSH_USER in ~/.camply")
	}
	return &pushoverNotifier{
		client: &http.Client{Timeout: 5 * time.Second},
		config: cfg,
	}, nil
}

func (p *pushoverNotifier) SendCampsites(campsites []core.AvailableCampsite) error {
	for _, c := range campsites {
		title, message := formatMessage(c)

		payload := map[string]interface{}{
			"token":   p.config.PushoverPushToken,
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
		defer resp.Body.Close()

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
		return nil, fmt.Errorf("Telegram requires both TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID in ~/.camply")
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
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("telegram API returned status: %d", resp.StatusCode)
		}
	}
	return nil
}

// RunTestNotifications executes the test flow just like the Python version
func RunTestNotifications(providers []string, cfg *config.AppConfig) error {
	var notifiers []Notifier

	for _, p := range providers {
		switch p {
		case "pushover":
			n, err := NewPushover(cfg)
			if err != nil {
				return err
			}
			notifiers = append(notifiers, n)
		case "telegram":
			n, err := NewTelegram(cfg)
			if err != nil {
				return err
			}
			notifiers = append(notifiers, n)
		default:
			fmt.Printf("⚠️  Warning: Provider '%s' is not yet implemented in the Go rewrite\n", p)
		}
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
