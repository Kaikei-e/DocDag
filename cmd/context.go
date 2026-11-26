package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/brief"
	"github.com/Kaikei-e/DocDag/internal/render"
	"github.com/Kaikei-e/DocDag/model"
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
		Long: "Assemble a reading brief for one document within a token budget.\n\n" +
			"<ref> accepts a document ID, filename, or managed file path. Instead of\n" +
			"reading multiple files manually, context returns the document itself, where\n" +
			"it resolves to over supersedes, its normative or typed-edge neighbours, and\n" +
			"an excerpt from each document.\n\n" +
			"By default, excerpts quote Decision (preferring Decision Outcome), falling\n" +
			"back to common decision/summary headings, the first non-metadata section, or\n" +
			"the lead paragraph. Pass --section to quote one specific heading strictly.",
		Example: "  docdag context 0002\n" +
			"  docdag context docs/adr/0002-store-thumbnails-on-disk.md --budget 600 --format md\n" +
			"  docdag context 0001 --depth 2 --all\n" +
			"  docdag context 0001 --section Consequences",
		Args: cobra.ExactArgs(1),
		RunE: runContext,
	}
	cmd.Flags().Int(flagDepth, brief.DefaultDepth, "how many typed edges to walk in each direction")
	cmd.Flags().StringArray(flagEdge, nil, "restrict the walk to an edge type (repeatable)")
	cmd.Flags().Int(flagBudget, brief.DefaultBudget, "token budget for the excerpts; 0 is unbounded")
	cmd.Flags().String(flagSection, "", "heading whose first paragraph is quoted (default: Decision, with fallback)")
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
	if flags.Changed(flagSection) {
		if opts.Section, err = flags.GetString(flagSection); err != nil {
			return usageErr("%v", err)
		}
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
			return unknownEdgeErr(cfg, edge)
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
