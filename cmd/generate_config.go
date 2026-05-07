package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kantis/dynatrace-tui/internal/config"
)

func generateConfigCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "generate-config",
		Short: "Write a starter config file with defaults and commented examples",
		Long: "Writes a YAML scaffold to the default config path " +
			"(~/.config/dynatrace-tui/config.yaml) or to the path given by --config. " +
			"Refuses to overwrite an existing file unless --force is set.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			path := configPath
			if path == "" {
				p, err := config.DefaultPath()
				if err != nil {
					return err
				}
				path = p
			}
			return writeStarterConfig(path, force)
		},
	}
	c.Flags().BoolVarP(&force, "force", "f", false, "overwrite an existing config file")
	return c
}

func writeStarterConfig(path string, force bool) error {
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("%s already exists; pass --force to overwrite", path)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	return nil
}

const starterConfig = `# dttui configuration. Edit the placeholders below before running dttui.
# Docs: https://github.com/kantis/dynatrace-tui#configuration
#
# Each entry under environments: is a Dynatrace tenant authenticated with
# a Platform Token. Get one at https://myaccount.dynatrace.com/platformTokens
# with scopes: storage:logs:read, storage:buckets:read.

environments:
  PROD:
    environment_id: REPLACE_ME    # the prefix in https://<env-id>.apps.dynatrace.com
    platform_token: REPLACE_ME    # dt0s16.XXXXXXXX.YYYY...

  # Add more environments by uncommenting and editing the block below.
  # TEST:
  #   environment_id: def67890
  #   platform_token: dt0s16.AAAA.BBBB

# Optional. Pin a starting environment by name. Defaults to the first one
# in the file.
# default: PROD

# Optional. Enables the vim-style modal query editor. Off by default.
# vim_mode: true

# Optional. When the selected record carries a structured 'msg' field, the
# detail pane defaults to showing only that payload (header reads
# "Details (simplified)"). Toggle full / simplified at runtime with Alt-D.
# Set to false to always render the full record.
# simplified_preview: true

# Optional. Override the Ctrl-T time-range preset lists. Omit the block
# (or either inner list) to use the built-in defaults. Each entry is
# either a now()-<duration> relative offset, a literal datetime, or one
# of these dynamic tokens (resolved when the modal opens):
#   start_of_hour  — the start of the current hour
#   start_of_day   — the start of the current day
#   now() / now    — the moment the modal opened
# time_picker:
#   from:
#     - now()-5m
#     - now()-30m
#     - now()-1h
#     - now()-12h
#     - now()-7d
#     - start_of_hour
#     - start_of_day
#   to:
#     - now()
`
