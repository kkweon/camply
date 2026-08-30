package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/logger"
	"github.com/kkweon/camply/internal/providers"
	"github.com/kkweon/camply/internal/providers/recdotgov"
)

// newVocabularyCmd builds `camply <provider> vocabulary`.
//
// An adapter maps an open, operator-authored vocabulary onto a closed domain,
// and left alone it rots — silently, because an unrecognised value degrades to
// Unknown and Unknown is the safe answer nobody investigates. The tests catch
// drift the moment a new value enters the repository as a fixture; this catches
// it the moment it enters the API.
//
// It is meant to be run as a slow CronJob beside the campsite searches, on the
// same notification channel: drift becomes a message rather than a decay.
func newVocabularyCmd(d providers.Descriptor, registry []providers.Descriptor) *cobra.Command {
	r := &vocabularyRunner{desc: d, registry: registry}

	cmd := &cobra.Command{
		Use:   "vocabulary",
		Short: "Report values " + d.DisplayName + " returns that camply does not map",
		Long: "Fetches the campgrounds given and reports any value this provider's adapter\n" +
			"does not recognise, by value, so it can be added. Exits non-zero when it finds\n" +
			"any, so a scheduled run turns vocabulary drift into a failed job rather than a\n" +
			"slow decay into Unknown.",
		RunE: r.run,
	}

	f := cmd.Flags()
	f.StringSliceVar(&r.campgrounds, "campgrounds", nil,
		multiHelp("Campground IDs to sample", "campgrounds 232461,10300216",
			"campgrounds 232461", "campgrounds 10300216"))
	f.StringSliceVar(&r.notifications, "notifications", nil,
		multiHelp("Notification providers", "notifications pushover,telegram",
			"notifications pushover", "notifications telegram"))

	return cmd
}

type vocabularyRunner struct {
	desc          providers.Descriptor
	registry      []providers.Descriptor
	campgrounds   []string
	notifications []string
}

func (r *vocabularyRunner) run(_ *cobra.Command, _ []string) error {
	logger.Camply("camply, checking %s's vocabulary 🔤", r.desc.DisplayName)

	if len(r.campgrounds) == 0 {
		return fmt.Errorf("no campgrounds given. Pass --campgrounds with the ones you search, "+
			"e.g. --campgrounds 232461,10300216\n"+
			"  Drift is per campground: each operator fills %s's fields differently",
			r.desc.DisplayName)
	}
	if r.desc.New == nil {
		return fmt.Errorf("%s has no implementation to check", r.desc.DisplayName)
	}

	// A month-wide window so the sample covers as many campsites as the API
	// will return; the search results themselves are thrown away.
	now := time.Now()
	req := core.SearchRequest{
		Campgrounds: r.campgrounds,
		StartDates:  []time.Time{now},
		EndDates:    []time.Time{now.AddDate(0, 1, 0)},
		Nights:      1,
	}

	recdotgov.TakeDrift() // ignore anything an earlier command left behind
	if _, err := r.desc.New().FindCampsites(context.Background(), req); err != nil {
		return fmt.Errorf("failed to sample %s: %w", r.desc.DisplayName, err)
	}

	report := recdotgov.TakeDrift()
	if len(report) == 0 {
		logger.Info("No unmapped values across %d campground(s). The adapter is current.",
			len(r.campgrounds))
		return nil
	}

	for _, line := range report {
		logger.Warn("%s", line)
	}
	return fmt.Errorf("%s returned %d kind(s) of value camply does not map — see above",
		r.desc.DisplayName, len(report))
}
