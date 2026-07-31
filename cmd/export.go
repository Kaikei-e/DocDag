package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/render"
)

const flagOut = "out"

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Render the typed graph as mermaid, DOT or node-link JSON",
		Args:  cobra.NoArgs,
		RunE:  runExport,
	}
	cmd.Flags().String(flagFormat, formatMermaid, "output format: mermaid|dot|json")
	cmd.Flags().Bool(flagIncludeRefs, false, "include the reference layer")
	cmd.Flags().String(flagOut, "-", "output file, or - for stdout")
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
	target, err := flags.GetString(flagOut)
	if err != nil {
		return usageErr("%v", err)
	}
	g, _, err := loadGraph(cmd)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if target != "" && target != "-" {
		file, err := os.Create(target)
		if err != nil {
			return ioErr(fmt.Errorf("create %s: %w", target, err))
		}
		defer func() { _ = file.Close() }()
		out = file
	}
	return writeGraph(out, format, g, opts)
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
