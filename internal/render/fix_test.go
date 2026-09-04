package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/model"
)

func TestFindingsTextIndentsTheFixUnderTheFinding(t *testing.T) {
	findings := []model.Finding{{
		Severity: model.SeverityError,
		Rule:     model.RuleStatusDrift,
		ID:       "0001",
		Detail:   "has inbound supersedes but status is not superseded",
		Location: model.Location{Path: "docs/adr/0001-a.md", Line: 3},
		Fix:      "set status: superseded in docs/adr/0001-a.md",
	}}

	var buf bytes.Buffer
	if err := FindingsText(&buf, findings, model.Summary{Errors: 1}, ""); err != nil {
		t.Fatalf("FindingsText: %v", err)
	}

	want := "docs/adr/0001-a.md:3: ERROR status_drift 0001: has inbound supersedes but status is not superseded\n" +
		"  fix: set status: superseded in docs/adr/0001-a.md\n"
	if buf.String() != want {
		t.Errorf("report = %q, want %q", buf.String(), want)
	}
}

func TestFindingsTextOmitsAnAbsentFix(t *testing.T) {
	findings := []model.Finding{{
		Severity: model.SeverityError,
		Rule:     model.RuleIDCollision,
		ID:       "0004",
		Detail:   "shares its identifier with 0004-b.md",
		Location: model.Location{Path: "0004-a.md", Line: 1},
	}}

	var buf bytes.Buffer
	if err := FindingsText(&buf, findings, model.Summary{Errors: 1}, ""); err != nil {
		t.Fatalf("FindingsText: %v", err)
	}

	if strings.Contains(buf.String(), "fix:") {
		t.Errorf("report = %q, want no fix line for a finding without one", buf.String())
	}
}
