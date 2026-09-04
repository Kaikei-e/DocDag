package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/internal/vcs"
)

// The two time flags. They are independent: --as-of moves the day the corpus is
// asked about and --at moves the revision it is read from, which are the valid
// time and the transaction time of a temporal database. Asking for both is a
// bitemporal question — what the vault at that revision said was in force on
// that day — and neither one changes what the other means.
const (
	flagAsOf = "as-of"
	flagAt   = "at"
	// envAsOf is where --as-of falls back to, so a scheduled run can set the
	// day once for a whole pipeline. The flag wins wherever both are written.
	envAsOf = "DOCDAG_AS_OF"
)

// addAsOfFlag declares the day a command answers for. defaulted names where the
// day comes from when nobody writes one, because the two answers — the day HEAD
// was committed on, and today — are the difference between a gate and a query.
func addAsOfFlag(cmd *cobra.Command, defaulted string) {
	cmd.Flags().String(flagAsOf, "",
		"the day to answer for, as YYYY-MM-DD (default: "+defaulted+"; $"+envAsOf+" is read too)")
}

// addAtFlag declares the revision a command reads its documents from. It is
// read-only: it changes which documents are read, never what is written, so no
// command that writes declares it.
func addAtFlag(cmd *cobra.Command) {
	cmd.Flags().String(flagAt, "", "read every managed document from this revision instead of the working tree")
}

// asOfToday resolves the day a query is about: what the caller asked for, and
// today where they asked for nothing. A listing is a question about now, and
// answering it about the last commit would be a surprise.
func asOfToday(cmd *cobra.Command) (time.Time, error) {
	return asOfDate(cmd, time.Now)
}

// asOfCommitted resolves the day a check is about: what the caller asked for,
// and the day HEAD was committed on where they asked for nothing.
//
// A gate has to answer the same way for one commit however long afterwards it
// runs — a corpus that passed yesterday and fails today without a commit is a
// gate nobody can act on — and the committer date is the one day a repository
// carries that does not move. Detecting an expiry as it happens is a scheduled
// run with --as-of $(date -I), which says out loud that it asks about today.
func asOfCommitted(cmd *cobra.Command) (time.Time, error) {
	return asOfDate(cmd, headDay)
}

// asOfDate reads the day from the flag, then from the environment, and falls
// back to what the command means by "no opinion".
func asOfDate(cmd *cobra.Command, fallback func() time.Time) (time.Time, error) {
	written, err := cmd.Flags().GetString(flagAsOf)
	if err != nil {
		return time.Time{}, usageErr("%v", err)
	}
	source := "--" + flagAsOf
	if written == "" {
		written, source = os.Getenv(envAsOf), envAsOf
	}
	written = strings.TrimSpace(written)
	if written == "" {
		return fallback(), nil
	}
	day, err := time.Parse(config.AttrDateLayout, written)
	if err != nil {
		return time.Time{}, usageErr("%s %q is not a date as YYYY-MM-DD", source, written)
	}
	return day, nil
}

// headDay reports the day HEAD was committed on. A corpus outside a repository,
// or a git that cannot answer, is not an error: there is no commit to pin the
// answer to, so the run is about today.
func headDay() time.Time {
	root, err := os.Getwd()
	if err != nil {
		return time.Now()
	}
	repo, err := vcs.Open(root)
	if err != nil {
		return time.Now()
	}
	day, err := repo.CommitterDate("HEAD")
	if err != nil {
		return time.Now()
	}
	committed, err := time.Parse(config.AttrDateLayout, day)
	if err != nil {
		return time.Now()
	}
	return committed
}

// corpus is what a read-only command works on: the graph, the configuration it
// was read under, and the revision it was read from — empty for the working
// tree. Closing it releases the temporary tree --at materialized, which has to
// outlive the parse: a brief quotes document bodies from disk.
type corpus struct {
	graph *model.Graph
	cfg   config.Config
	at    string
	tree  *vcs.Tree
}

func (c *corpus) close() {
	if c != nil && c.tree != nil {
		_ = c.tree.Close()
	}
}

// loadCorpus resolves the configuration and reads the documents it describes,
// from the working tree or from the revision --at names.
func loadCorpus(cmd *cobra.Command) (*corpus, error) {
	cfg, err := effectiveConfig(cmd)
	if err != nil {
		return nil, err
	}
	rev, err := revision(cmd)
	if err != nil {
		return nil, err
	}
	if rev != "" {
		return loadCorpusAt(cfg, rev)
	}
	g, cfg, err := loadGraph(cmd)
	if err != nil {
		return nil, err
	}
	return &corpus{graph: g, cfg: cfg}, nil
}

// revision reads the --at flag, and reports none for a command that does not
// declare it.
func revision(cmd *cobra.Command) (string, error) {
	if cmd.Flags().Lookup(flagAt) == nil {
		return "", nil
	}
	rev, err := cmd.Flags().GetString(flagAt)
	if err != nil {
		return "", usageErr("%v", err)
	}
	return rev, nil
}

// loadCorpusAt reads every managed document from a revision. The committed
// files are written into a temporary tree and read by the same parser the
// working tree is read by, so --at changes which documents are read and nothing
// about how they are checked. A corpus outside a repository cannot answer at
// all, which is a setup error rather than a failing corpus.
func loadCorpusAt(cfg config.Config, rev string) (*corpus, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, ioErr(fmt.Errorf("working directory: %w", err))
	}
	repo, err := vcs.Open(root)
	if err != nil {
		return nil, ioErr(fmt.Errorf("--%s: %w", flagAt, err))
	}
	tree, err := repo.Checkout(rev, cfg.DocumentDirs())
	if err != nil {
		return nil, ioErr(fmt.Errorf("--%s: %w", flagAt, err))
	}
	docs, err := parse.Documents(cfg.Reroot(tree.Dirs()))
	if err != nil {
		_ = tree.Close()
		return nil, ioErr(err)
	}
	// The tree holds the repository's own layout, so naming the files relative
	// to its root names them the way the revision holds them: a report about
	// v1.2.0 points at paths a reader finds in v1.2.0.
	parse.Localize(docs, tree.Root())
	return &corpus{graph: graph.Build(docs, cfg), cfg: cfg, at: rev, tree: tree}, nil
}

// reportedAsOf is the as-of day a text report carries, and the empty string
// where it carries none. Only a corpus whose kinds declare a period has an
// answer that depends on the day, and only there does a person reading the text
// report need to be told which day it was: everywhere else the line would be
// noise in front of a report that means the same on every day.
//
// The JSON reports carry it either way — a consumer that reads a field is not
// reading a line — which is where this deliberately departs from ADR-0005 D5.
func reportedAsOf(cfg config.Config, asOf time.Time) string {
	if !cfg.Periods() {
		return ""
	}
	return graph.AsOfDay(asOf)
}
