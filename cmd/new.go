package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/newdoc"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/internal/render"
)

const (
	flagSupersedes = "supersedes"
	flagDependsOn  = "depends-on"
	flagDryRun     = "dry-run"
	flagID         = "id"
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
	cmd.Flags().String(flagID, "", "identifier to create the document under, instead of the next free one")
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
	id, err := flags.GetString(flagID)
	if err != nil {
		return usageErr("%v", err)
	}
	g, cfg, err := loadGraph(cmd)
	if err != nil {
		return err
	}
	// Which kind to create, under which identity rules and from which template,
	// is a question a multi-kind corpus has no default answer to, and guessing
	// one would write a document into the wrong directory.
	if cfg.Multikind() {
		return domainErr("new requires --kind on a multi-kind corpus; per-kind templates arrive with the spec preset")
	}
	root, err := os.Getwd()
	if err != nil {
		return ioErr(fmt.Errorf("working directory: %w", err))
	}
	req := newdoc.Request{ID: id, Title: args[0], Supersedes: supersedes, DependsOn: dependsOn}
	plan, err := newdoc.NewPlan(g, cfg, req)
	if err != nil {
		return creationErr(err)
	}
	out := cmd.OutOrStdout()

	if dryRun {
		if err := render.CreationPlan(out, planReport(plan, root), cfg.StatusField, format == formatJSON); err != nil {
			return ioErr(err)
		}
		return nil
	}
	path, err := plan.Apply()
	if err != nil {
		return creationErr(err)
	}
	if err := render.CreatedPath(out, parse.LocalPath(root, path), format == formatJSON); err != nil {
		return ioErr(err)
	}
	return nil
}

// creationErr keeps what the corpus says a domain failure and everything else
// an I/O one, whether it surfaced while planning or while writing.
func creationErr(err error) error {
	if errors.Is(err, model.ErrUnknownID) || errors.Is(err, model.ErrIDConflict) {
		return domainErr("%v", err)
	}
	return ioErr(fmt.Errorf("create document: %w", err))
}

// planReport names every file a plan touches the way a caller standing in root
// would type it, so a plan reads like the findings a validation prints.
func planReport(plan newdoc.Plan, root string) render.Plan {
	out := render.Plan{
		ID:       plan.ID,
		Path:     parse.LocalPath(root, plan.Path),
		Exists:   plan.Exists,
		Rewrites: make([]render.PlanRewrite, 0, len(plan.Rewrites)),
	}
	for _, r := range plan.Rewrites {
		out.Rewrites = append(out.Rewrites, render.PlanRewrite{Path: parse.LocalPath(root, r.Path), Status: r.Status})
	}
	return out
}
