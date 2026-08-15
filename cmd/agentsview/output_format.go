package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type formatFlag string

func (f *formatFlag) String() string { return string(*f) }

func (f *formatFlag) Set(v string) error {
	switch v {
	case "human", "json":
		*f = formatFlag(v)
		return nil
	default:
		return errors.New("must be human or json")
	}
}

func (*formatFlag) Type() string { return "human|json" }

func registerFormatFlags(flags *pflag.FlagSet) {
	f := formatFlag("human")
	flags.Var(&f, "format", "Output format: human or json")
	flags.Bool("json", false, "Emit JSON output (alias for --format json)")
}

func rejectFormatFlags(cmd *cobra.Command, cmdName, streams string) error {
	if cmd.Flags().Changed("format") || cmd.Flags().Changed("json") {
		return fmt.Errorf(
			"%s: streams %s; --format not supported (--json alias also unsupported)",
			cmdName, streams,
		)
	}
	return nil
}

func outputFormat(cmd *cobra.Command) string {
	if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
		return "json"
	}
	if f := cmd.Flag("format"); f != nil {
		return f.Value.String()
	}
	return "human"
}
