package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/lint"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// LintSchemaVersion is the version of the JSON lint report.
const LintSchemaVersion = 1

// LintKind is the top-level kind of the lint report. It is a different word
// from a validation report's, because the two answer about different things —
// the configuration and the corpus — and a consumer must never read one as the
// other.
const LintKind = "lint"

// LintReport is the JSON shape of `docdag lint`.
type LintReport struct {
	Kind          string          `json:"kind"`
	SchemaVersion int             `json:"schema_version"`
	PresetVersion int             `json:"preset_version,omitempty"`
	Findings      []model.Finding `json:"findings"`
	Summary       lint.Summary    `json:"summary"`
}

// LintText writes one line per lint finding, in the shape a validation finding
// is written in, and a closing line saying what the run found.
func LintText(w io.Writer, findings []model.Finding, summary lint.Summary) error {
	out := &errWriter{w: w}
	for _, f := range findings {
		out.printf("%s%s %s%s: %s\n", locationPrefix(f.Location), strings.ToUpper(string(f.Severity)), f.Rule, subject(f.ID), f.Detail)
		if f.Fix != "" {
			out.printf("  fix: %s\n", f.Fix)
		}
	}
	out.printf("%s\n", lintSummary(summary))
	if out.err != nil {
		return fmt.Errorf("write lint findings: %w", out.err)
	}
	return nil
}

// lintSummary is the closing line: what the run found, or that it found
// nothing. A lint run that reports nothing has said something worth printing —
// the configuration was read and holds up — where a run that reports findings
// ends with the count the exit code was decided on.
func lintSummary(summary lint.Summary) string {
	if summary.Errors == 0 && summary.Warnings == 0 && summary.Infos == 0 {
		return "OK: no lint findings"
	}
	counts := []string{
		fmt.Sprintf("%d %s", summary.Errors, plural(summary.Errors, "error")),
		fmt.Sprintf("%d %s", summary.Warnings, plural(summary.Warnings, "warning")),
	}
	if summary.Infos > 0 {
		counts = append(counts, fmt.Sprintf("%d info", summary.Infos))
	}
	return strings.Join(counts, ", ")
}

func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// LintJSON writes the lint findings as a JSON report of their own kind.
func LintJSON(w io.Writer, findings []model.Finding, summary lint.Summary, presetVersion int) error {
	report := LintReport{
		Kind:          LintKind,
		SchemaVersion: LintSchemaVersion,
		PresetVersion: presetVersion,
		Findings:      findings,
		Summary:       summary,
	}
	if report.Findings == nil {
		report.Findings = []model.Finding{}
	}
	if err := writeJSON(w, report); err != nil {
		return fmt.Errorf("write lint findings: %w", err)
	}
	return nil
}

// LintGitHub writes one workflow command per lint finding, then the closing
// line. An info finding is a notice, which no job fails on.
func LintGitHub(w io.Writer, findings []model.Finding, summary lint.Summary) error {
	out := &errWriter{w: w}
	writeAnnotations(out, findings)
	out.printf("%s\n", lintSummary(summary))
	if out.err != nil {
		return fmt.Errorf("write lint findings: %w", out.err)
	}
	return nil
}

// LintRDJSON writes the lint findings in the reviewdog diagnostic format.
func LintRDJSON(w io.Writer, findings []model.Finding, summary lint.Summary) error {
	worst := model.SeverityError
	switch {
	case summary.Errors > 0:
	case summary.Warnings > 0:
		worst = model.SeverityWarn
	default:
		worst = model.SeverityInfo
	}
	return writeRDJSON(w, findings, worst)
}

// FixturePaths writes the files a generated fixture put on disk, one per line
// or as the JSON object a machine reads them from. A generation that wrote
// nothing still answers: the fixture was already there.
func FixturePaths(w io.Writer, paths []string, asJSON bool) error {
	if asJSON {
		if paths == nil {
			paths = []string{}
		}
		if err := writeJSON(w, map[string][]string{"paths": paths}); err != nil {
			return fmt.Errorf("write fixture paths: %w", err)
		}
		return nil
	}
	out := &errWriter{w: w}
	for _, path := range paths {
		out.printf("%s\n", path)
	}
	if out.err != nil {
		return fmt.Errorf("write fixture paths: %w", out.err)
	}
	return nil
}
