package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/render"
	"github.com/Kaikei-e/DocDag/model"
)

const (
	flagOut       = "out"
	flagConnected = "connected"
)

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Render the typed graph as mermaid, DOT or node-link JSON",
		Args:  cobra.NoArgs,
		RunE:  runExport,
	}
	cmd.Flags().String(flagFormat, formatMermaid, "output format: mermaid|dot|json")
	cmd.Flags().Bool(flagIncludeRefs, false, "include the reference layer")
	cmd.Flags().Bool(flagConnected, false, "emit only documents a typed edge touches")
	cmd.Flags().StringArray(flagEdge, nil, "restrict the typed edges to one type (repeatable)")
	cmd.Flags().String(flagOut, "-", "output file, or - for stdout")
	addAtFlag(cmd)
	return cmd
}

func runExport(cmd *cobra.Command, _ []string) error {
	flags := cmd.Flags()
	format, err := flags.GetString(flagFormat)
	if err != nil {
		return usageErr("%v", err)
	}
	if err := validFormat(format, formatMermaid, formatDOT, formatJSON); err != nil {
		return err
	}
	opts := render.Options{LabelLimit: render.LabelLimit}
	if opts.IncludeRefs, err = flags.GetBool(flagIncludeRefs); err != nil {
		return usageErr("%v", err)
	}
	if opts.Connected, err = flags.GetBool(flagConnected); err != nil {
		return usageErr("%v", err)
	}
	edges, err := flags.GetStringArray(flagEdge)
	if err != nil {
		return usageErr("%v", err)
	}
	target, err := flags.GetString(flagOut)
	if err != nil {
		return usageErr("%v", err)
	}
	c, err := loadCorpus(cmd)
	if err != nil {
		return err
	}
	defer c.close()
	g, cfg := c.graph, c.cfg
	for _, name := range edges {
		if _, ok := cfg.Edge(model.EdgeType(name)); !ok {
			return usageErr("unknown edge type %q", name)
		}
		opts.Edges = append(opts.Edges, model.EdgeType(name))
	}

	if target == "" || target == "-" {
		return writeGraph(cmd.OutOrStdout(), format, g, opts)
	}
	file, err := os.Create(target)
	if err != nil {
		return ioErr(fmt.Errorf("create %s: %w", target, err))
	}
	if err := writeGraph(file, format, g, opts); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return ioErr(fmt.Errorf("close %s: %w", target, err))
	}
	return nil
}

func writeGraph(out io.Writer, format string, g *model.Graph, opts render.Options) error {
	var err error
	switch format {
	case formatDOT:
		err = render.DOT(out, g, opts)
	case formatJSON:
		err = render.NodeLinkJSON(out, g, opts)
	default:
		err = render.Mermaid(out, g, opts)
	}
	if err != nil {
		return ioErr(err)
	}
	return nil
}
