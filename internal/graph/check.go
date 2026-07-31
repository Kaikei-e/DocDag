package graph

import (
	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// CheckDocuments reports the file-level structural findings that the graph
// container cannot express: id collisions, undecodable and absent frontmatter.
func CheckDocuments(docs []*parse.Document, cfg config.Config) []model.Finding { return nil }

// CheckCycles reports one finding per cycle found in an acyclic edge type.
func CheckCycles(g *model.Graph, cfg config.Config) []model.Finding { return nil }

// CheckDangling reports typed edges whose target is not a known document.
func CheckDangling(g *model.Graph, cfg config.Config) []model.Finding { return nil }

// CheckStatusVocabulary reports statuses outside the configured vocabulary.
func CheckStatusVocabulary(g *model.Graph, cfg config.Config) []model.Finding { return nil }

// CheckDerived reports derived edges that contradict the structured edges and
// warns wherever a derived edge stands in for structured frontmatter.
func CheckDerived(g *model.Graph, cfg config.Config) []model.Finding { return nil }

// Check runs every built-in structural check. These cannot be disabled.
func Check(g *model.Graph, cfg config.Config) []model.Finding { return nil }

// MatchCondition reports whether one node satisfies every clause of a rule
// condition.
func MatchCondition(g *model.Graph, cond config.Condition, id model.ID) bool { return false }

// EvalRule evaluates one declarative rule over every node.
func EvalRule(g *model.Graph, rule config.Rule) []model.Finding { return nil }

// EvalRules evaluates every configured rule over every node.
func EvalRules(g *model.Graph, cfg config.Config) []model.Finding { return nil }

// Validate runs the structural checks and the configured rules, returning the
// findings already recorded on the graph too, in deterministic order.
func Validate(g *model.Graph, cfg config.Config) []model.Finding { return nil }

// SortFindings orders findings by severity, then rule, then id, then detail.
func SortFindings(findings []model.Finding) {}

// Summarize counts documents, typed edges and findings for the summary line.
func Summarize(g *model.Graph, findings []model.Finding) model.Summary {
	return model.Summary{}
}
