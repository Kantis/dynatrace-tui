package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/kantis/dynatrace-tui/internal/auth"
	"github.com/kantis/dynatrace-tui/internal/config"
	"github.com/kantis/dynatrace-tui/internal/grail"
)

func queryCmd() *cobra.Command {
	var timeframe string
	c := &cobra.Command{
		Use:   "query [DQL]",
		Short: "Run a DQL query and print result records as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuery(cmd.Context(), args[0], timeframe)
		},
	}
	c.Flags().StringVarP(&timeframe, "timeframe", "t", "", "convenience preset (15m, 1h, 6h, 24h); ignored if DQL already has from:")
	return c
}

var validTimeframes = map[string]struct{}{
	"15m": {}, "1h": {}, "6h": {}, "24h": {},
}

func applyTimeframe(dql, tf string) (string, error) {
	if tf == "" {
		return dql, nil
	}
	if _, ok := validTimeframes[tf]; !ok {
		return "", fmt.Errorf("invalid --timeframe %q (allowed: 15m, 1h, 6h, 24h)", tf)
	}
	if strings.Contains(dql, "from:") {
		return dql, nil
	}
	// Inject `, from:now()-<tf>` after the first verb token.
	// Simplest: if the query starts with "fetch <something>", insert after that token.
	// For PoC accuracy, just append: `<dql> | filter timestamp > now()-<tf>` is safer
	// when caller built a complex query, but DQL prefers `from:`. We'll insert into the
	// fetch clause if it's recognizable, otherwise append a filter.
	trimmed := strings.TrimSpace(dql)
	if strings.HasPrefix(trimmed, "fetch ") {
		head, tail := trimmed, ""
		if i := strings.IndexAny(trimmed[len("fetch "):], ",|"); i >= 0 {
			head = strings.TrimRight(trimmed[:len("fetch ")+i], " \t")
			tail = trimmed[len("fetch ")+i:]
		}
		injected := head + ", from:now()-" + tf
		if tail != "" {
			injected += " " + strings.TrimLeft(tail, " \t")
		}
		return injected, nil
	}
	return trimmed + " | filter timestamp > now()-" + tf, nil
}

func runQuery(parent context.Context, dql, timeframe string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	finalDQL, err := applyTimeframe(dql, timeframe)
	if err != nil {
		return err
	}

	var tokens grail.TokenProvider
	if cfg.PlatformToken != "" {
		tokens = auth.Static(cfg.PlatformToken)
	} else {
		tokens = auth.New(cfg.ClientID, cfg.ClientSecret, cfg.Scopes)
	}
	client := grail.New(cfg.EnvironmentID, tokens)

	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintln(os.Stderr, "executing query...")
	exec, err := client.Execute(ctx, grail.ExecuteRequest{Query: finalDQL})
	if err != nil {
		return err
	}

	final := exec
	if exec.State == grail.StateRunning {
		final, err = client.PollUntilDone(ctx, exec.RequestToken)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				cancelCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
				defer c()
				if cerr := client.Cancel(cancelCtx, exec.RequestToken); cerr != nil {
					fmt.Fprintln(os.Stderr, "cancel:", cerr)
				} else {
					fmt.Fprintln(os.Stderr, "cancelled")
				}
			}
			return err
		}
	}

	switch final.State {
	case grail.StateSucceeded:
		records := grail.Records{}
		if final.Result != nil && final.Result.Records != nil {
			records = final.Result.Records
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(records)
	case grail.StateCancelled:
		return fmt.Errorf("query cancelled")
	case grail.StateFailed:
		if final.Error != nil {
			return final.Error
		}
		return fmt.Errorf("query failed")
	default:
		return fmt.Errorf("unexpected final state %q", final.State)
	}
}
