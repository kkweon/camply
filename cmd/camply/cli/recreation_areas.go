package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/providers/recdotgov"
	"github.com/kkweon/camply/internal/providers/usedirect"
)

var (
	raSearchStr string
	raStateStr  string
)

var recreationAreasCmd = &cobra.Command{
	Use:   "recreation-areas",
	Short: "Search for Recreation Areas",
	RunE: func(cmd *cobra.Command, args []string) error {
		req := core.SearchRequest{
			Query: raSearchStr,
			State: raStateStr,
		}

		var provider interface {
			FindRecreationAreas(context.Context, core.SearchRequest) ([]core.RecreationArea, error)
		}

		switch providerStr {
		case "RecreationDotGov":
			provider = recdotgov.NewProvider()
		case "ReserveCalifornia":
			provider = usedirect.NewProvider("ReserveCalifornia", "https://california-rdr.prod.cali.rd12.recreation-management.tylerapp.com")
		default:
			return fmt.Errorf("unsupported or missing provider: %s", providerStr)
		}

		fmt.Printf("🏕  Searching %s for Recreation Areas...\n", providerStr)

		ctx := context.Background()
		areas, err := provider.FindRecreationAreas(ctx, req)
		if err != nil {
			return fmt.Errorf("error fetching recreation areas: %w", err)
		}

		printRecreationAreasTable(areas)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(recreationAreasCmd)

	recreationAreasCmd.Flags().StringVar(&providerStr, "provider", "RecreationDotGov", "Camping Search Provider")
	recreationAreasCmd.Flags().StringVar(&raSearchStr, "search", "", "Search string")
	recreationAreasCmd.Flags().StringVar(&raStateStr, "state", "", "State abbreviation")
}

func printRecreationAreasTable(areas []core.RecreationArea) {
	if len(areas) == 0 {
		fmt.Println("❌ No recreation areas found matching your criteria.")
		return
	}

	for _, a := range areas {
		fmt.Printf("🏞  %s - (#%s)\n", a.RecreationArea, a.RecreationAreaID)
		fmt.Printf("    📍 Location: %s\n", a.RecreationAreaLocation)
	}

	fmt.Printf("\n✅ Found %d matching recreation area(s)!\n", len(areas))
}
