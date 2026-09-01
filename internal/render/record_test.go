package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
)

func testRecords() []Record {
	return []Record{
		{ID: "0001", Title: "Cache thumbnails", Status: "accepted", Path: "docs/adr/0001-cache.md"},
		{ID: "0002", Title: "Store on disk", Status: "superseded", Path: "docs/adr/0002-disk.md", Reference: true},
	}
}

func TestRecordsTextDefaultsToTheIdentifierAlone(t *testing.T) {
	var buf bytes.Buffer
	if err := RecordsText(&buf, testRecords(), nil); err != nil {
		t.Fatalf("RecordsText: %v", err)
	}

	want := "0001\n0002 (reference)\n"
	if buf.String() != want {
		t.Errorf("records = %q, want %q", buf.String(), want)
	}
}

func TestRecordsTextSeparatesTheChosenFieldsWithTabs(t *testing.T) {
	var buf bytes.Buffer
	if err := RecordsText(&buf, testRecords(), []string{FieldPath, FieldID, FieldStatus}); err != nil {
		t.Fatalf("RecordsText: %v", err)
	}

	want := "docs/adr/0001-cache.md\t0001\taccepted\ndocs/adr/0002-disk.md\t0002\tsuperseded (reference)\n"
	if buf.String() != want {
		t.Errorf("records = %q, want %q", buf.String(), want)
	}
}

func TestRecordsJSONOmitsTheReferenceMarkerOnTypedHits(t *testing.T) {
	var buf bytes.Buffer
	if err := RecordsJSON(&buf, testRecords()[:1]); err != nil {
		t.Fatalf("RecordsJSON: %v", err)
	}

	got := buf.String()
	for _, want := range []string{`"id": "0001"`, `"title": "Cache thumbnails"`, `"status": "accepted"`, `"path": "docs/adr/0001-cache.md"`} {
		if !strings.Contains(got, want) {
			t.Errorf("record = %s, want it to contain %s", got, want)
		}
	}
	if strings.Contains(got, "reference") {
		t.Errorf("record = %s, want no reference marker on a typed hit", got)
	}
}

func TestRecordsJSONWritesAnEmptyArray(t *testing.T) {
	var buf bytes.Buffer
	if err := RecordsJSON(&buf, nil); err != nil {
		t.Fatalf("RecordsJSON: %v", err)
	}

	if strings.TrimSpace(buf.String()) != "[]" {
		t.Errorf("records = %q, want an empty array", buf.String())
	}
}

func TestRecordsDescribeTheDocuments(t *testing.T) {
	g := testOKBasicGraph()

	got := Records(g, []model.ID{"0001"})

	if len(got) != 1 {
		t.Fatalf("records = %+v, want one", got)
	}
	if got[0].Title == "" || got[0].Path == "" || got[0].Status == "" {
		t.Errorf("record = %+v, want the document described", got[0])
	}
}

func TestQueryRecordsMarkTheReferenceLayer(t *testing.T) {
	g := testOKBasicGraph()

	got := QueryRecords(g, []graph.QueryResult{
		{ID: "0001", Layer: graph.LayerTyped},
		{ID: "0002", Layer: graph.LayerReference},
	})

	if len(got) != 2 {
		t.Fatalf("records = %+v, want two", got)
	}
	if got[0].Reference {
		t.Errorf("record = %+v, want a typed hit unmarked", got[0])
	}
	if !got[1].Reference {
		t.Errorf("record = %+v, want a reference-layer hit marked", got[1])
	}
}

func TestRecordRenderersPropagateWriterErrors(t *testing.T) {
	if err := RecordsText(failingWriter{}, testRecords(), nil); err == nil {
		t.Error("RecordsText err = nil, want the write failure surfaced")
	}
	if err := RecordsJSON(failingWriter{}, testRecords()); err == nil {
		t.Error("RecordsJSON err = nil, want the write failure surfaced")
	}
}

func TestWithProjectionsAddsTheDerivedColumns(t *testing.T) {
	g := testOKBasicGraph()
	cfg := config.ADRPreset()
	records := Records(g, []model.ID{"0002", "0003"})

	got := WithProjections(records, []string{config.ProjectionAcceptedUnsuperseded}, graph.EvalProjections(g, cfg))

	var buf bytes.Buffer
	if err := RecordsText(&buf, got, []string{FieldID, config.ProjectionAcceptedUnsuperseded}); err != nil {
		t.Fatalf("RecordsText: %v", err)
	}
	want := "0002\tfalse\n0003\ttrue\n"
	if buf.String() != want {
		t.Fatalf("records = %q, want %q", buf.String(), want)
	}
}

func TestWithProjectionsLeavesAListingThatAskedForNoneAlone(t *testing.T) {
	g := testOKBasicGraph()
	records := Records(g, []model.ID{"0002"})

	got := WithProjections(records, nil, graph.EvalProjections(g, config.ADRPreset()))

	if got[0].Projections != nil {
		t.Fatalf("projections = %v, want none", got[0].Projections)
	}
}
