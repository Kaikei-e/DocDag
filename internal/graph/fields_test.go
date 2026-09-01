package graph

import (
	"slices"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// The lifecycle the deprecation tests are written against: a field retired in
// revision 2 whose value moves to another key, and a sunset a test can stand on
// either side of.
const (
	testRetiredField = "owner"
	testSunset       = "2027-01-01"
)

// testDeprecatedConfig retires one frontmatter field, leaving everything else
// the ADR preset says.
func testDeprecatedConfig(spec config.FieldSpec) config.Config {
	cfg := config.ADRPreset()
	cfg.Fields = map[string]config.FieldSpec{testRetiredField: spec}
	return cfg
}

// testDeprecatedGraph builds a one-document corpus writing the retired field.
// The document goes through the parser's own key-line bookkeeping, so a
// location assertion is about the line a reader would open.
func testDeprecatedGraph(cfg config.Config) *model.Graph {
	return Build([]*parse.Document{
		testDoc("0001", map[string]any{
			"title":          "Store events in an append-only table",
			"status":         config.StatusAccepted,
			testRetiredField: "platform",
		}, ""),
	}, cfg)
}

func testDay(t *testing.T, day string) time.Time {
	t.Helper()
	parsed, err := time.Parse(config.AttrDateLayout, day)
	if err != nil {
		t.Fatalf("parse %q: %v", day, err)
	}
	return parsed
}

func TestCheckDeprecatedFields(t *testing.T) {
	t.Run("a retired field warns, on the line it is written on", func(t *testing.T) {
		cfg := testDeprecatedConfig(config.FieldSpec{
			Deprecated: true, Since: 2, MigrateTo: "owned-by", Sunset: testSunset,
		})
		g := testDeprecatedGraph(cfg)

		got := CheckDeprecatedFields(g, cfg, testDay(t, "2026-09-01"))

		f := testAssertSingleFinding(t, got, model.RuleDeprecatedField, model.SeverityWarn, "0001")
		want := `frontmatter key "owner" is deprecated since preset version 2, sunset 2027-01-01`
		if f.Detail != want {
			t.Errorf("detail = %q, want %q", f.Detail, want)
		}
		// testDoc numbers the keys in sorted order from the delimiter, so owner
		// is the first key of title, status and owner.
		if f.Location != (model.Location{Path: "0001.md", Line: testFrontmatterLine + 1}) {
			t.Errorf("location = %+v, want the owner key's line", f.Location)
		}
	})

	t.Run("past its sunset the same field is an error", func(t *testing.T) {
		cfg := testDeprecatedConfig(config.FieldSpec{Deprecated: true, Since: 2, Sunset: testSunset})
		g := testDeprecatedGraph(cfg)

		got := CheckDeprecatedFields(g, cfg, testDay(t, "2027-01-02"))

		f := testAssertSingleFinding(t, got, model.RuleDeprecatedField, model.SeverityError, "0001")
		want := `frontmatter key "owner" is deprecated since preset version 2, past its sunset 2027-01-01`
		if f.Detail != want {
			t.Errorf("detail = %q, want %q", f.Detail, want)
		}
	})

	t.Run("the sunset day itself is still the last day it warns", func(t *testing.T) {
		cfg := testDeprecatedConfig(config.FieldSpec{Deprecated: true, Sunset: testSunset})
		g := testDeprecatedGraph(cfg)

		got := CheckDeprecatedFields(g, cfg, testDay(t, testSunset))

		testAssertSingleFinding(t, got, model.RuleDeprecatedField, model.SeverityWarn, "0001")
	})

	t.Run("a declaration without a sunset never escalates", func(t *testing.T) {
		cfg := testDeprecatedConfig(config.FieldSpec{Deprecated: true})
		g := testDeprecatedGraph(cfg)

		got := CheckDeprecatedFields(g, cfg, testDay(t, "2099-12-31"))

		f := testAssertSingleFinding(t, got, model.RuleDeprecatedField, model.SeverityWarn, "0001")
		if f.Detail != `frontmatter key "owner" is deprecated` {
			t.Errorf("detail = %q, want the bare deprecation", f.Detail)
		}
	})

	t.Run("a caller naming no day is asking about today", func(t *testing.T) {
		// The zero time is the default the CLI overrides with the day it runs
		// on; a sunset long past is an error whichever day today is.
		past := testDeprecatedConfig(config.FieldSpec{Deprecated: true, Sunset: "2000-01-01"})
		future := testDeprecatedConfig(config.FieldSpec{Deprecated: true, Sunset: "2999-01-01"})

		testAssertSingleFinding(t, CheckDeprecatedFields(testDeprecatedGraph(past), past, time.Time{}),
			model.RuleDeprecatedField, model.SeverityError, "0001")
		testAssertSingleFinding(t, CheckDeprecatedFields(testDeprecatedGraph(future), future, time.Time{}),
			model.RuleDeprecatedField, model.SeverityWarn, "0001")
	})

	t.Run("a field the configuration declares but does not retire reports nothing", func(t *testing.T) {
		cfg := testDeprecatedConfig(config.FieldSpec{})
		g := testDeprecatedGraph(cfg)

		if got := CheckDeprecatedFields(g, cfg, testDay(t, "2027-06-01")); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("a retired field nobody writes reports nothing", func(t *testing.T) {
		cfg := testDeprecatedConfig(config.FieldSpec{Deprecated: true, Sunset: "2000-01-01"})
		g := Build([]*parse.Document{
			testDoc("0001", map[string]any{"status": config.StatusAccepted}, ""),
		}, cfg)

		if got := CheckDeprecatedFields(g, cfg, testDay(t, "2027-06-01")); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: the migration is what finishes the check", got)
		}
	})

	t.Run("structural raises the deprecation before its sunset", func(t *testing.T) {
		cfg := testDeprecatedConfig(config.FieldSpec{Deprecated: true, Sunset: testSunset})
		cfg.Structural = map[string]model.Severity{model.RuleDeprecatedField: model.SeverityError}
		g := testDeprecatedGraph(cfg)

		testAssertSingleFinding(t, CheckDeprecatedFields(g, cfg, testDay(t, "2026-09-01")),
			model.RuleDeprecatedField, model.SeverityError, "0001")
	})

	t.Run("findings come back sorted", func(t *testing.T) {
		cfg := testDeprecatedConfig(config.FieldSpec{Deprecated: true})
		g := Build([]*parse.Document{
			testDoc("0002", map[string]any{"status": config.StatusAccepted, testRetiredField: "platform"}, ""),
			testDoc("0001", map[string]any{"status": config.StatusAccepted, testRetiredField: "platform"}, ""),
		}, cfg)

		got := CheckDeprecatedFields(g, cfg, testDay(t, "2026-09-01"))

		if len(got) != 2 {
			t.Fatalf("findings = %+v, want one per document", got)
		}
		testAssertSortedFindings(t, got)
	})
}

func TestCheckDeprecatedFieldsPerKind(t *testing.T) {
	cfg := testKindsConfig()
	cfg.Fields = map[string]config.FieldSpec{testRetiredField: {Deprecated: true, MigrateTo: "owned-by"}}
	cfg.Kinds["conform"] = config.KindSpec{
		Dir: "spec/conform", ID: `^conform/[a-z0-9-]+$`,
		// The kind still uses the key, so for its documents it is not retired.
		Fields: map[string]config.FieldSpec{testRetiredField: {}},
	}
	docs := []*parse.Document{
		testKindDoc("clause", "UZ-V-001", map[string]any{"status": config.StatusAccepted, testRetiredField: "platform"}),
		testKindDoc("conform", "conform/check", map[string]any{"status": config.StatusAccepted, testRetiredField: "platform"}),
	}
	g := Build(docs, cfg)

	got := CheckDeprecatedFields(g, cfg, testDay(t, "2026-09-01"))

	t.Run("a kind that re-declares the field keeps it", func(t *testing.T) {
		testAssertSingleFinding(t, got, model.RuleDeprecatedField, model.SeverityWarn, "UZ-V-001")
	})

	t.Run("a closed kind knows the declared field", func(t *testing.T) {
		// A retired field is a declared one: reporting it as unknown would call
		// the migration a mistake.
		if unknown := testFindingsFor(g.Findings, model.RuleUnknownField); len(unknown) != 0 {
			t.Fatalf("unknown_field = %+v, want the declared field accepted by the closed kind", unknown)
		}
	})

	t.Run("validate reports the deprecation on the closed kind", func(t *testing.T) {
		findings := Validate(g, cfg, testDay(t, "2026-09-01"))

		if !slices.Contains(testRuleNames(findings), model.RuleDeprecatedField) {
			t.Fatalf("rules = %v, want the deprecation among them", testRuleNames(findings))
		}
	})
}

func TestSuggestMigratesARetiredField(t *testing.T) {
	t.Run("a declaration naming a successor says where the value goes", func(t *testing.T) {
		cfg := testDeprecatedConfig(config.FieldSpec{Deprecated: true, MigrateTo: "owned-by"})
		g := testDeprecatedGraph(cfg)

		got := Suggest(CheckDeprecatedFields(g, cfg, testDay(t, "2026-09-01")), g, cfg)

		if len(got) != 1 || got[0].Fix != "migrate owner to owned-by" {
			t.Fatalf("fix = %+v, want \"migrate owner to owned-by\"", got)
		}
	})

	t.Run("a field being removed rather than moved has no mechanical remedy", func(t *testing.T) {
		cfg := testDeprecatedConfig(config.FieldSpec{Deprecated: true})
		g := testDeprecatedGraph(cfg)

		got := Suggest(CheckDeprecatedFields(g, cfg, testDay(t, "2026-09-01")), g, cfg)

		if len(got) != 1 || got[0].Fix != "" {
			t.Fatalf("fix = %+v, want none", got)
		}
	})
}
