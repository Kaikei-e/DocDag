package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/newdoc"
)

const (
	flagSupersedes = "supersedes"
	flagDependsOn  = "depends-on"
)

func newNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <title>",
		Short: "Create the next document and mark the ones it supersedes",
		Args:  cobra.ExactArgs(1),
		RunE:  runNew,
	}
	cmd.Flags().StringArray(flagSupersedes, nil, "reference this document supersedes (repeatable)")
	cmd.Flags().StringArray(flagDependsOn, nil, "reference this document depends on (repeatable)")
	return cmd
}

func runNew(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	supersedes, err := flags.GetStringArray(flagSupersedes)
	if err != nil {
		return usageErr("%v", err)
	}
	dependsOn, err := flags.GetStringArray(flagDependsOn)
	if err != nil {
		return usageErr("%v", err)
	}
	g, cfg, err := loadGraph(cmd)
	if err != nil {
		return err
	}
	req := newdoc.Request{Title: args[0], Supersedes: supersedes, DependsOn: dependsOn}
	path, err := newdoc.Create(g, cfg, req)
	if err != nil {
		return ioErr(fmt.Errorf("create document: %w", err))
	}
	fmt.Fprintln(cmd.OutOrStdout(), path)
	return nil
}
