package cmd

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/render"
	"github.com/Kaikei-e/DocDag/model"
)

type ruleCoverageEntry struct {
	rule      string
	fixture   string // path relative to testdata/fixtures/
	coveredBy string // test name if covered elsewhere without a fixture
}

// ruleCoverageTable maps every structural rule, preset rule, dangling_reference,
// and immutable_violation to a fixture that makes it fire, or to a test name
// covering it.
var ruleCoverageTable = []ruleCoverageEntry{
	// Structural rules (from config.StructuralRules())
	{rule: model.RuleCardinality, fixture: "cardinality"},
	{rule: model.RuleCycle, fixture: "cycle"},
	{rule: model.RuleDanglingRef, fixture: "dangling"},
	{rule: model.RuleDeprecatedField, fixture: "rules/deprecated_field"},
	{rule: model.RuleDerivedConflict, fixture: "rules/derived_conflict"},
	{rule: model.RuleEdgeAttrInvalid, fixture: "edge-attrs"},
	{rule: model.RuleEdgeAttrMissing, fixture: "edge-attrs"},
	{rule: model.RuleEdgeAttrUnknown, fixture: "edge-attrs"},
	{rule: model.RuleEdgeKindMismatch, fixture: "kinds"},
	{rule: model.RuleEmptyEdge, fixture: "empty-edge"},
	{rule: model.RuleExceptsStrict, fixture: "rules/excepts_strict"},
	{rule: model.RuleExpiredDeviation, fixture: "spec-vault"},
	{rule: model.RuleIDCollision, fixture: "id-collision"},
	{rule: model.RuleIDMismatch, fixture: "kinds"},
	{rule: model.RuleInvalidFrontmatter, fixture: "invalid-yaml"},
	{rule: model.RuleInvalidRef, fixture: "rules/invalid_ref"},
	{rule: model.RuleInverseMismatch, fixture: "inverse-mismatch"},
	{rule: model.RuleKindMismatch, fixture: "kinds"},
	{rule: model.RuleMissingField, fixture: "rules/missing_field"},
	{rule: model.RuleMissingFrontmatter, fixture: "rules/missing_frontmatter"},
	{rule: model.RuleMissingSection, fixture: "sections"},
	{rule: model.RuleModalityConflict, fixture: "spec-vault"},
	{rule: model.RulePaddingMismatch, fixture: "rules/padding_mismatch"},
	{rule: model.RulePathMismatch, fixture: "path-constraints"},
	{rule: model.RulePeriodConflict, fixture: "rules/period_conflict"},
	{rule: model.RulePeriodInvalid, fixture: "rules/period_invalid"},
	{rule: model.RuleSectionOrder, fixture: "sections"},
	{rule: model.RuleStaleTarget, fixture: "target"},
	{rule: model.RuleUnknownField, fixture: "kinds"},
	{rule: model.RuleUnknownFieldValue, fixture: "rules/unknown_field_value"},
	{rule: model.RuleUnknownStatus, fixture: "rules/unknown_status"},
	{rule: model.RuleUnmanagedFile, fixture: "rules/unmanaged_file"},
	{rule: model.RuleUnstructuredSupersedes, fixture: "ok-madr"},

	// Preset rules (from config.PresetRules())
	{rule: model.RuleDeviationPressure, fixture: "rules/deviation_pressure"},
	{rule: model.RuleInteropNotMust, fixture: "rules/interop_not_must"},
	{rule: model.RuleMayWithoutInterop, fixture: "rules/may_without_interop"},
	{rule: model.RuleNoCounterexample, fixture: "spec-vault"},
	{rule: model.RuleOrphanMust, fixture: "spec-vault"},
	{rule: model.RuleOrphanTest, fixture: "spec-vault"},
	{rule: model.RulePendingSuccessor, fixture: "spec-vault"},
	{rule: model.RulePrematureSuperseded, fixture: "rules/premature_superseded"},
	{rule: model.RuleStalePremise, fixture: "spec-vault"},
	{rule: model.RuleStatusDrift, fixture: "status-drift"},
	{rule: model.RuleSupersededOrphan, fixture: "superseded-orphan"},

	// Explicit additional rules
	{rule: model.RuleDanglingReference, fixture: "dangling-reference"},
	{rule: model.RuleImmutableViolation, coveredBy: "TestValidateImmutableSinceOnAppendOnlyKinds"},
}

var (
	coverageCacheMu sync.Mutex
	coverageCache   = make(map[string]render.Report)
)

func runCoverageFixture(t *testing.T, fixtureRelPath string) render.Report {
	t.Helper()

	coverageCacheMu.Lock()
	if report, ok := coverageCache[fixtureRelPath]; ok {
		coverageCacheMu.Unlock()
		return report
	}
	coverageCacheMu.Unlock()

	fixtureDir := filepath.Join(fixturesRoot, fixtureRelPath)
	if _, err := os.Stat(fixtureDir); err != nil {
		t.Fatalf("fixture directory not found: %s", fixtureDir)
	}

	cfgFile := filepath.Join(fixtureDir, "docdag.yaml")
	var args []string
	if _, err := os.Stat(cfgFile); err == nil {
		data, err := os.ReadFile(cfgFile)
		if err != nil {
			t.Fatalf("read config %s: %v", cfgFile, err)
		}
		var cfg config.Config
		_ = yaml.Unmarshal(data, &cfg)
		if len(cfg.Kinds) > 0 || cfg.Preset == config.PresetSpec {
			args = []string{"validate", "--format", "json", "--config", cfgFile}
		} else {
			args = []string{"validate", "--format", "json", "--dir", fixtureDir, "--config", cfgFile}
		}
	} else {
		args = []string{"validate", "--format", "json", "--dir", fixtureDir}
	}

	got := run(t, args...)
	report := decodeJSON[render.Report](t, got.stdout)

	coverageCacheMu.Lock()
	coverageCache[fixtureRelPath] = report
	coverageCacheMu.Unlock()

	return report
}

// expectedRules returns all rule names that must be present in the coverage table:
// every rule returned by StructuralRules(), every preset rule, dangling_reference,
// and immutable_violation.
func expectedRules() []string {
	rules := make(map[string]bool)
	for _, r := range config.StructuralRules() {
		rules[r] = true
	}
	for _, p := range config.Presets() {
		for _, r := range config.PresetRules(p) {
			rules[r] = true
		}
	}
	rules[model.RuleDanglingReference] = true
	rules[model.RuleImmutableViolation] = true
	return slices.Sorted(maps.Keys(rules))
}

func TestRuleCoverage(t *testing.T) {
	tableMap := make(map[string]ruleCoverageEntry, len(ruleCoverageTable))
	for _, entry := range ruleCoverageTable {
		tableMap[entry.rule] = entry
	}

	// Completeness guard: fails if any rule has no table entry or no fixture/coverage.
	for _, rule := range expectedRules() {
		entry, ok := tableMap[rule]
		if !ok || (entry.fixture == "" && entry.coveredBy == "") {
			t.Errorf("rule %q has no fixture that makes it fire", rule)
		}
	}

	// Run every table entry to prove the rule actually fires.
	for _, entry := range ruleCoverageTable {
		t.Run(entry.rule, func(t *testing.T) {
			if entry.coveredBy != "" {
				// Covered by an existing test; handled with t.Skip-free bookkeeping.
				t.Logf("rule %q covered by %s", entry.rule, entry.coveredBy)
				return
			}
			if entry.fixture == "" {
				t.Fatalf("rule %q has no fixture that makes it fire", entry.rule)
			}

			report := runCoverageFixture(t, entry.fixture)
			found := false
			for _, f := range report.Findings {
				if f.Rule == entry.rule {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("rule %q did not fire in fixture %q; findings were: %+v", entry.rule, entry.fixture, report.Findings)
			}
		})
	}
}
