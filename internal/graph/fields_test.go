package graph

import (
	"slices"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/model"
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

		got := Suggest(CheckDeprecatedFields(g, cfg, testDay(t, "2026-09-01")), g, cfg, testAsOf)

		if len(got) != 1 || got[0].Fix != "migrate owner to owned-by" {
			t.Fatalf("fix = %+v, want \"migrate owner to owned-by\"", got)
		}
	})

	t.Run("a field being removed rather than moved has no mechanical remedy", func(t *testing.T) {
		cfg := testDeprecatedConfig(config.FieldSpec{Deprecated: true})
		g := testDeprecatedGraph(cfg)

		got := Suggest(CheckDeprecatedFields(g, cfg, testDay(t, "2026-09-01")), g, cfg, testAsOf)

		if len(got) != 1 || got[0].Fix != "" {
			t.Fatalf("fix = %+v, want none", got)
		}
	})
}

// The vocabulary the value tests are written against: a field a document has to
// write, from a closed set of two.
var testModalitySpec = config.FieldSpec{OneOf: []string{"MUST", "SHOULD"}, Required: true}

// testFieldValueGraph builds a one-document corpus writing the frontmatter
// handed in, under a configuration that declares the vocabulary. The document
// goes through the parser's own key-line bookkeeping, so a location assertion
// is about the line a reader would open.
func testFieldValueGraph(frontmatter map[string]any) (*model.Graph, config.Config) {
	cfg := config.ADRPreset()
	cfg.Fields = map[string]config.FieldSpec{"modality": testModalitySpec}
	return Build([]*parse.Document{testDoc("0001", frontmatter, "")}, cfg), cfg
}

func TestCheckFieldValues(t *testing.T) {
	t.Run("a value outside the vocabulary is reported on the key's line", func(t *testing.T) {
		g, cfg := testFieldValueGraph(map[string]any{
			"title": "Store events in an append-only table", "status": config.StatusAccepted, "modality": "SHALL",
		})

		got := CheckFieldValues(g, cfg)

		f := testAssertSingleFinding(t, got, model.RuleUnknownFieldValue, model.SeverityError, "0001")
		if want := `modality "SHALL" is outside the vocabulary MUST, SHOULD`; f.Detail != want {
			t.Errorf("detail = %q, want %q", f.Detail, want)
		}
		// testDoc numbers the keys in sorted order from the delimiter, so
		// modality is the first of modality, status and title.
		if f.Location != (model.Location{Path: "0001.md", Line: testFrontmatterLine + 1}) {
			t.Errorf("location = %+v, want the modality key's line", f.Location)
		}
	})

	t.Run("a value inside it reports nothing", func(t *testing.T) {
		g, cfg := testFieldValueGraph(map[string]any{"status": config.StatusAccepted, "modality": "MUST"})

		if got := CheckFieldValues(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("the comparison is exact: a closed vocabulary is not prose", func(t *testing.T) {
		g, cfg := testFieldValueGraph(map[string]any{"status": config.StatusAccepted, "modality": "must"})

		testAssertSingleFinding(t, CheckFieldValues(g, cfg), model.RuleUnknownFieldValue, model.SeverityError, "0001")
	})

	t.Run("a required key nobody wrote is a finding of its own", func(t *testing.T) {
		g, cfg := testFieldValueGraph(map[string]any{
			"title": "Store events in an append-only table", "status": config.StatusAccepted,
		})

		got := CheckFieldValues(g, cfg)

		f := testAssertSingleFinding(t, got, model.RuleMissingField, model.SeverityError, "0001")
		if want := `frontmatter key "modality" is required, one of: MUST, SHOULD`; f.Detail != want {
			t.Errorf("detail = %q, want %q", f.Detail, want)
		}
		// There is no line of its own to point at, so the finding lands on the
		// status: the reader is being sent to a document, not to a mistake.
		if f.Location != (model.Location{Path: "0001.md", Line: testFrontmatterLine + 1}) {
			t.Errorf("location = %+v, want the status key's line", f.Location)
		}
	})

	t.Run("a required key holding a list is a key holding no value", func(t *testing.T) {
		g, cfg := testFieldValueGraph(map[string]any{
			"status": config.StatusAccepted, "modality": []any{"MUST", "SHOULD"},
		})

		testAssertSingleFinding(t, CheckFieldValues(g, cfg), model.RuleMissingField, model.SeverityError, "0001")
	})

	t.Run("a field the configuration only declares constrains nothing", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.Fields = map[string]config.FieldSpec{"owner": {}}
		g := Build([]*parse.Document{
			testDoc("0001", map[string]any{"status": config.StatusAccepted, "owner": "platform"}, ""),
		}, cfg)

		if got := CheckFieldValues(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: a declaration without a vocabulary is a known key and nothing more", got)
		}
	})

	t.Run("a kind's declaration describes its own documents alone", func(t *testing.T) {
		// The vocabulary is the clause kind's, so the conformance test beside
		// it is not missing anything, closed or open.
		cfg := testKindsConfig()
		cfg.Kinds["clause"] = config.KindSpec{
			Dir: "spec/clauses", ID: `^UZ-[A-Z]-\d{3}$`, Closed: true,
			Fields: map[string]config.FieldSpec{"modality": testModalitySpec},
		}
		g := Build([]*parse.Document{
			testKindDoc("clause", "UZ-V-001", map[string]any{"status": config.StatusAccepted}),
			testKindDoc("conform", "conform/uz-v-001", map[string]any{"enforces": []any{"UZ-V-001"}}),
		}, cfg)

		got := CheckFieldValues(g, cfg)

		testAssertSingleFinding(t, got, model.RuleMissingField, model.SeverityError, "UZ-V-001")
	})

	t.Run("validate reports both, and structural cannot lower them", func(t *testing.T) {
		g, cfg := testFieldValueGraph(map[string]any{"status": config.StatusAccepted})

		if !slices.Contains(testRuleNames(Validate(g, cfg, time.Time{})), model.RuleMissingField) {
			t.Fatalf("rules = %v, want the missing field among them", testRuleNames(Validate(g, cfg, time.Time{})))
		}
		cfg.Structural = map[string]model.Severity{model.RuleUnknownFieldValue: model.SeverityWarn}
		if err := cfg.Validate(); err == nil {
			t.Error("Validate accepted a lowered unknown_field_value, want a configuration error")
		}
	})
}
