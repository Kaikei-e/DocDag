package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/brief"
	"github.com/Kaikei-e/DocDag/model"
	"github.com/Kaikei-e/DocDag/internal/render"
)

const (
	flagDepth   = "depth"
	flagBudget  = "budget"
	flagSection = "section"
	flagAll     = "all"
)

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context <ref>",
		Short: "Print a document, where it resolves to and its typed-edge neighbourhood",
		Args:  cobra.ExactArgs(1),
		RunE:  runContext,
	}
	cmd.Flags().Int(flagDepth, brief.DefaultDepth, "how many typed edges to walk in each direction")
	cmd.Flags().StringArray(flagEdge, nil, "restrict the walk to an edge type (repeatable)")
	cmd.Flags().Int(flagBudget, brief.DefaultBudget, "token budget for the excerpts; 0 is unbounded")
	cmd.Flags().String(flagSection, brief.DefaultSection, "heading whose first paragraph is quoted")
	cmd.Flags().Bool(flagAll, false, "include documents that are not binding")
	addAsOfFlag(cmd, "today")
	addAtFlag(cmd)
	return cmd
}

func runContext(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(cmd, formatText, formatJSON, formatMarkdown)
	if err != nil {
		return err
	}
	flags := cmd.Flags()
	opts := brief.Options{}
	if opts.Depth, err = flags.GetInt(flagDepth); err != nil {
		return usageErr("%v", err)
	}
	if opts.Budget, err = flags.GetInt(flagBudget); err != nil {
		return usageErr("%v", err)
	}
	if opts.Section, err = flags.GetString(flagSection); err != nil {
		return usageErr("%v", err)
	}
	if opts.All, err = flags.GetBool(flagAll); err != nil {
		return usageErr("%v", err)
	}
	edges, err := flags.GetStringArray(flagEdge)
	if err != nil {
		return usageErr("%v", err)
	}

	if opts.AsOf, err = asOfToday(cmd); err != nil {
		return err
	}
	c, err := loadCorpus(cmd)
	if err != nil {
		return err
	}
	defer c.close()
	g, cfg := c.graph, c.cfg
	opts.At = c.at
	for _, edge := range edges {
		if _, ok := cfg.Edge(model.EdgeType(edge)); !ok {
			return usageErr("unknown edge type %q", edge)
		}
		opts.Types = append(opts.Types, model.EdgeType(edge))
	}
	id, err := normalize(g, cfg, args[0])
	if err != nil {
		return err
	}
	b, err := brief.Build(g, cfg, id, opts)
	if err != nil {
		return ioErr(err)
	}

	out := cmd.OutOrStdout()
	switch format {
	case formatJSON:
		err = render.ContextJSON(out, b)
	case formatMarkdown:
		err = render.ContextMarkdown(out, b)
	default:
		err = render.ContextText(out, b)
	}
	if err != nil {
		return ioErr(err)
	}
	return nil
}
