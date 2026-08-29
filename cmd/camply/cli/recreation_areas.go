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
	desc     *providers.Descriptor

	provider string
	search   string
	state    string
}

// newProviderRecreationAreasCmd builds `camply <provider> recreation-areas`.
func newProviderRecreationAreasCmd(d providers.Descriptor) *cobra.Command {
	r := &recAreasRunner{desc: &d}

	cmd := &cobra.Command{
		Use:   "recreation-areas",
		Short: "Search for recreation areas on " + d.DisplayName,
		RunE:  r.run,
	}

	addRecAreaLookupFlags(cmd, r, &d)

	return cmd
}

func addRecAreaLookupFlags(cmd *cobra.Command, r *recAreasRunner, d *providers.Descriptor) {
	f := cmd.Flags()

	if d == nil {
		f.StringVar(&r.provider, "provider", "RecreationDotGov", "Camping Search Provider")
	}
	f.StringVar(&r.search, "search", "", "Search string (one value)")
	if d == nil || d.SupportsState {
		f.StringVar(&r.state, "state", "", "State abbreviation (one value)")
	}
}

func (r *recAreasRunner) run(cmd *cobra.Command, _ []string) error {
	req := core.SearchRequest{
		Query: r.search,
		State: r.state,
	}

	provider, desc, err := r.resolveProvider()
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

func (r *recAreasRunner) resolveProvider() (providers.Provider, providers.Descriptor, error) {
	if r.desc != nil {
		return r.desc.New(), *r.desc, nil
	}
	return providers.NewFrom(r.registry, r.provider)
}
