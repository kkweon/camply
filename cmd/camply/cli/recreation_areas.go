package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/providers"
)

type recAreasRunner struct {
	registry []providers.Descriptor

	provider string
	search   string
	state    string
}

func newRecreationAreasCmd(descs []providers.Descriptor) *cobra.Command {
	r := &recAreasRunner{registry: descs}

	cmd := &cobra.Command{
		Use:   "recreation-areas",
		Short: "Search for Recreation Areas",
		RunE:  r.run,
	}

	cmd.Flags().StringVar(&r.provider, "provider", "RecreationDotGov", "Camping Search Provider")
	cmd.Flags().StringVar(&r.search, "search", "", "Search string")
	cmd.Flags().StringVar(&r.state, "state", "", "State abbreviation")

	return cmd
}

func (r *recAreasRunner) run(cmd *cobra.Command, _ []string) error {
	req := core.SearchRequest{
		Query: r.search,
		State: r.state,
	}

	provider, desc, err := providers.NewFrom(r.registry, r.provider)
	if err != nil {
		return err
	}

	fmt.Printf("🏕  Searching %s for Recreation Areas...\n", desc.DisplayName)

	areas, err := provider.FindRecreationAreas(context.Background(), req)
	if err != nil {
		return fmt.Errorf("error fetching recreation areas: %w", err)
	}

	printRecreationAreasTable(areas)
	return nil
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
