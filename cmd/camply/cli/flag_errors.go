package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kkweon/camply/internal/providers"
)

// providerScopedFlags are flags that exist on some providers and not others,
// paired with the test for whether a provider has one.
//
// Cobra's own "unknown flag: --state" says what failed but not what to do
// instead, which is the failure mode this whole change is about.
var providerScopedFlags = map[string]struct {
	supportedBy func(providers.Descriptor) bool
	because     string
}{
	"state": {
		supportedBy: func(d providers.Descriptor) bool { return d.SupportsState },
		because:     "its API has no state filter",
	},
}

var (
	unknownFlagRe   = regexp.MustCompile(`unknown flag: --([\w-]+)`)
	unknownShortcut = regexp.MustCompile(`unknown shorthand flag: '(\w)'`)
)

// unknownFlagName pulls the flag name out of cobra's parse error. Cobra offers
// no structured error here, so the message is all there is to work with; an
// unrecognised shape simply falls through to the original error.
func unknownFlagName(err error) string {
	if m := unknownFlagRe.FindStringSubmatch(err.Error()); m != nil {
		return m[1]
	}
	if m := unknownShortcut.FindStringSubmatch(err.Error()); m != nil {
		return m[1]
	}
	return ""
}

// providerFor reports which provider group a command sits under, if any.
func providerFor(cmd *cobra.Command, descs []providers.Descriptor) (providers.Descriptor, bool) {
	for c := cmd; c != nil; c = c.Parent() {
		if d, ok := providers.LookupIn(descs, c.Name()); ok {
			return d, true
		}
	}
	return providers.Descriptor{}, false
}

// providerAwareFlagError turns an unknown-flag error into one that names the
// providers where the flag does exist, and shows the command to run instead.
func providerAwareFlagError(descs []providers.Descriptor) func(*cobra.Command, error) error {
	return func(cmd *cobra.Command, err error) error {
		name := unknownFlagName(err)
		if name == "" {
			return err
		}

		scoped, isScoped := providerScopedFlags[name]
		if !isScoped {
			return err
		}

		current, onProvider := providerFor(cmd, descs)
		if !onProvider {
			return err
		}

		var supporters []providers.Descriptor
		for _, d := range descs {
			if d.Status == providers.StatusSupported && scoped.supportedBy(d) {
				supporters = append(supporters, d)
			}
		}
		if len(supporters) == 0 {
			return err
		}

		var b strings.Builder
		fmt.Fprintf(&b, "--%s is not available on %s.\n", name, current.Key)
		fmt.Fprintf(&b, "%s does not support it: %s.\n\n", current.DisplayName, scoped.because)

		fmt.Fprintf(&b, "These providers do support --%s:\n", name)
		for _, d := range supporters {
			fmt.Fprintf(&b, "    camply %s %s --%s ...\n", d.Key, cmd.Name(), name)
		}

		return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
	}
}
