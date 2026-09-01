package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/render"
	"github.com/Kaikei-e/DocDag/internal/vcs"
)

func newStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Report degree-based statistics for the document corpus",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := outputFormat(cmd, formatText, formatJSON)
			if err != nil {
				return err
			}
			fields, err := cmd.Flags().GetBool(flagFields)
			if err != nil {
				return usageErr("%v", err)
			}
			asOf, err := asOfToday(cmd)
			if err != nil {
				return err
			}
			c, err := loadCorpus(cmd)
			if err != nil {
				return err
			}
			defer c.close()
			g, cfg := c.graph, c.cfg
			out := cmd.OutOrStdout()
			if fields {
				// The field report is about frontmatter rather than degrees, so
				// it answers for a corpus that declares no supersedes edge too.
				usage := graph.ComputeFieldUsage(g, cfg, documentDates(g, cfg))
				if format == formatJSON {
					err = render.FieldUsageJSON(out, usage)
				} else {
					err = render.FieldUsageText(out, usage)
				}
				if err != nil {
					return ioErr(err)
				}
				return nil
			}
			// The binding count and the chain-depth distribution are supersedes
			// statistics; without that edge type they would be fiction.
			if err := requireSupersedes(cfg); err != nil {
				return err
			}
			stats := graph.ComputeStats(g, cfg, asOf)
			switch format {
			case formatJSON:
				err = render.StatsJSON(out, stats, render.Header{AsOf: graph.AsOfDay(asOf), At: c.at})
			default:
				err = render.StatsText(out, stats, reportedAsOf(cfg, asOf))
			}
			if err != nil {
				return ioErr(err)
			}
			return nil
		},
	}
	cmd.Flags().Bool(flagFields, false, "report frontmatter field usage instead of the degree statistics")
	addAsOfFlag(cmd, "today")
	addAtFlag(cmd)
	return cmd
}

// documentDates maps each document's path onto the day its file last changed in
// git. A corpus outside a repository, or a git that cannot answer, reports no
// dates at all: stats is a report rather than a check, and a date nobody can
// supply is a dash in the table.
func documentDates(g *model.Graph, cfg config.Config) map[string]string {
	dirs := corpusDirs(cfg)
	if len(dirs) == 0 {
		return nil
	}
	repo, err := vcs.Open(dirs[0])
	if err != nil {
		return nil
	}
	changed, err := repo.LastChanged(dirs...)
	if err != nil {
		return nil
	}
	dates := make(map[string]string, len(g.Nodes))
	for _, n := range g.Nodes {
		// A finding names a document the way the caller would type it, and git
		// names it relative to the working tree; the two meet here.
		rel, err := repo.Rel(n.Path)
		if err != nil {
			continue
		}
		if day, ok := changed[rel]; ok {
			dates[n.Path] = day
		}
	}
	return dates
}

// corpusDirs names the directories the documents live in: the one a single-kind
// corpus reads, or one per declared kind.
func corpusDirs(cfg config.Config) []string {
	if !cfg.Multikind() {
		if cfg.Dir == "" {
			return nil
		}
		return []string{cfg.Dir}
	}
	dirs := make([]string, 0, len(cfg.Kinds))
	for _, name := range cfg.KindNames() {
		dirs = append(dirs, cfg.Kinds[name].Dir)
	}
	return dirs
}
