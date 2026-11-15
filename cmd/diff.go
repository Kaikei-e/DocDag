package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/render"
)

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <rev>",
		Short: "Compare the typed graph at a base revision against the working tree",
		Args:  cobra.ExactArgs(1),
		RunE:  runDiff,
	}
	addAsOfFlag(cmd, "today")
	addAtFlag(cmd)
	return cmd
}

func runDiff(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(cmd, formatText, formatJSON, formatGitHub)
	if err != nil {
		return err
	}
	asOf, err := asOfToday(cmd)
	if err != nil {
		return err
	}
	target, err := loadCorpus(cmd)
	if err != nil {
		return err
	}
	defer target.close()

	base, err := loadCorpusAt(target.cfg, args[0])
	if err != nil {
		return err
	}
	defer base.close()

	diff := graph.Diff(base.graph, target.graph, target.cfg, asOf)
	out := cmd.OutOrStdout()
	switch format {
	case formatJSON:
		header := render.Header{
			PresetVersion: target.cfg.PresetVersion,
			AsOf:          graph.AsOfDay(asOf),
			At:            target.at,
		}
		err = render.DiffJSON(out, diff, header)
	case formatGitHub:
		err = render.DiffGitHub(out, diff)
	default:
		err = render.DiffText(out, diff)
	}
	if err != nil {
		return ioErr(err)
	}
	return nil
}
