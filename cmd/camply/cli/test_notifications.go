package cli

import (
	"fmt"

	"github.com/kkweon/camply/internal/config"
	notifications_pkg "github.com/kkweon/camply/internal/notifications"
	"github.com/spf13/cobra"
)

/*
   Recreation Area (e.g. Yosemite National Park)
         |
         |-- Contains -- 1...N
         v
    Campground (e.g. Upper Pines Campground)
         |
         |-- Contains -- 1...N
         v
     Campsite (e.g. Site 044, RV length 24)
*/

var testNotificationsCmd = &cobra.Command{
	Use:   "test-notifications",
	Short: "Test your notification provider setup",
	RunE: func(cmd *cobra.Command, args []string) error {
		notifications, _ := cmd.Flags().GetStringSlice("notifications")

		if len(notifications) == 0 {
			return fmt.Errorf("Missing option '--notifications'. Choose from: pushover, email, ntfy, apprise, pushbullet, slack, telegram, twilio, webhook, silent")
		}

		appConfig, err := config.Load()
		if err != nil {
			fmt.Printf("⚠️ Warning: Could not load ~/.camply config: %v\n", err)
		} else {
			fmt.Println("✅ Successfully loaded ~/.camply config")
			if appConfig.PushoverPushToken != "" {
				fmt.Println("   - Pushover credentials detected")
			}
			if appConfig.TelegramBotToken != "" {
				fmt.Println("   - Telegram credentials detected")
			}
		}

		// Execute the actual API calls based on ~/.camply configs
		if err := notifications_pkg.RunTestNotifications(notifications, appConfig); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(testNotificationsCmd)
	testNotificationsCmd.Flags().StringSlice("notifications", []string{}, "Notification providers to test")
}
