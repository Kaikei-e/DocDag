package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/newdoc"
	"github.com/Kaikei-e/DocDag/internal/render"
)

const (
	flagSupersedes = "supersedes"
	flagDependsOn  = "depends-on"
	flagDryRun     = "dry-run"
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
	cmd.Flags().Bool(flagDryRun, false, "print what would be written and write nothing")
	return cmd
}

func runNew(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(cmd, formatText, formatJSON)
	if err != nil {
		return err
	}
	flags := cmd.Flags()
	supersedes, err := flags.GetStringArray(flagSupersedes)
	if err != nil {
		return usageErr("%v", err)
	}
	dependsOn, err := flags.GetStringArray(flagDependsOn)
	if err != nil {
		return usageErr("%v", err)
	}
	dryRun, err := flags.GetBool(flagDryRun)
	if err != nil {
		return usageErr("%v", err)
	}
	g, cfg, err := loadGraph(cmd)
	if err != nil {
		return err
	}
	plan, err := newdoc.NewPlan(g, cfg, newdoc.Request{Title: args[0], Supersedes: supersedes, DependsOn: dependsOn})
	if err != nil {
		return creationErr(err)
	}
	out := cmd.OutOrStdout()

	if dryRun {
		if err := render.CreationPlan(out, planReport(plan), cfg.StatusField, format == formatJSON); err != nil {
			return ioErr(err)
		}
		return nil
	}
	path, err := plan.Apply()
	if err != nil {
		return creationErr(err)
	}
	if err := render.CreatedPath(out, path, format == formatJSON); err != nil {
		return ioErr(err)
	}
	return nil
}

// creationErr keeps an unknown reference a domain failure and everything else
// an I/O one, whether it surfaced while planning or while writing.
func creationErr(err error) error {
	if errors.Is(err, model.ErrUnknownID) {
		return domainErr("%v", err)
	}
	return ioErr(fmt.Errorf("create document: %w", err))
}

func planReport(plan newdoc.Plan) render.Plan {
	out := render.Plan{ID: plan.ID, Path: plan.Path, Rewrites: make([]render.PlanRewrite, 0, len(plan.Rewrites))}
	for _, r := range plan.Rewrites {
		out.Rewrites = append(out.Rewrites, render.PlanRewrite{Path: r.Path, Status: r.Status})
	}
	return out
}
