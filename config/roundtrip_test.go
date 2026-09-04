package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/Kaikei-e/DocDag/model"
)

// TestPresetYAMLRoundTrip holds each built-in preset to a marshal → load →
// DeepEqual cycle, and to byte-identical marshals across two calls. A
// configuration assembled in Go and written as docdag.yaml has to read back as
// the same value, and the bytes have to be deterministic so a CI regenerator
// can diff them.
func TestPresetYAMLRoundTrip(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  Config
	}{
		{name: "adr", cfg: ADRPreset()},
		{name: "spec", cfg: SpecPreset()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			first, err := yaml.Marshal(tt.cfg)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			second, err := yaml.Marshal(tt.cfg)
			if err != nil {
				t.Fatalf("Marshal again: %v", err)
			}
			if string(first) != string(second) {
				t.Fatalf("Marshal is not deterministic:\n--- first ---\n%s\n--- second ---\n%s", first, second)
			}

			path := filepath.Join(t.TempDir(), DefaultConfigFile)
			if err := os.WriteFile(path, first, 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("Validate after Load: %v", err)
			}
			if !reflect.DeepEqual(got, tt.cfg) {
				t.Fatalf("round-trip DeepEqual failed\n--- got ---\n%+v\n--- want ---\n%+v", got, tt.cfg)
			}
		})
	}
}

func TestEdgeConditionYAMLRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want EdgeCondition
	}{
		{
			name: "scalar stays scalar",
			src:  "inbound: supersedes\n",
			want: EdgeCondition{Edge: "supersedes"},
		},
		{
			name: "default min collapses to scalar",
			src:  "inbound: {edge: supersedes, min: 1}\n",
			want: EdgeCondition{Edge: "supersedes"},
		},
		{
			name: "raised min stays a mapping",
			src:  "inbound: {edge: supersedes, min: 5}\n",
			want: EdgeCondition{Edge: "supersedes", Min: testInt(5)},
		},
		{
			name: "max stays a mapping",
			src:  "inbound: {edge: supersedes, max: 1}\n",
			want: EdgeCondition{Edge: "supersedes", Max: testInt(1)},
		},
		{
			name: "both bounds stay a mapping",
			src:  "inbound:\n  edge: supersedes\n  min: 2\n  max: 4\n",
			want: EdgeCondition{Edge: "supersedes", Min: testInt(2), Max: testInt(4)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var first Condition
			if err := yaml.Unmarshal([]byte(tt.src), &first); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			out, err := yaml.Marshal(first)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if tt.want.scalarForm() {
				if strings.Contains(string(out), "edge:") {
					t.Fatalf("Marshal = %q, want the scalar form", out)
				}
			} else if !strings.Contains(string(out), "edge:") {
				t.Fatalf("Marshal = %q, want the mapping form", out)
			}
			var second Condition
			if err := yaml.Unmarshal(out, &second); err != nil {
				t.Fatalf("re-Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(second.Inbound, tt.want) {
				t.Fatalf("inbound = %+v, want %+v", second.Inbound, tt.want)
			}
		})
	}
}

// TestConfigYAMLTagsCoverExportedFields refuses a new exported field that
// cannot survive a YAML round trip. A field without a yaml tag is silently
// dropped by Marshal and would make DeepEqual on presets pass while the
// generated file lost the value.
func TestConfigYAMLTagsCoverExportedFields(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var missing []string
	var walk func(prefix string, typ reflect.Type)
	walk = func(prefix string, typ reflect.Type) {
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		switch typ.Kind() {
		case reflect.Struct:
			if seen[typ] {
				return
			}
			seen[typ] = true
			if typ.PkgPath() != "" && !strings.HasPrefix(typ.PkgPath(), "github.com/Kaikei-e/DocDag/") {
				return
			}
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				if !field.IsExported() {
					continue
				}
				name := prefix + "." + field.Name
				if field.Tag.Get("yaml") == "" && field.Type != reflect.TypeFor[model.Severity]() {
					// model.Severity is a string alias used as a map value; the
					// map itself carries the yaml tag on Config.Structural.
					if typ == reflect.TypeFor[Config]() || typ.PkgPath() == "github.com/Kaikei-e/DocDag/config" {
						missing = append(missing, name)
					}
				}
				walk(name, field.Type)
			}
		case reflect.Slice, reflect.Array, reflect.Map:
			walk(prefix+"[]", typ.Elem())
			if typ.Kind() == reflect.Map {
				walk(prefix+"{}", typ.Key())
			}
		}
	}
	walk("Config", reflect.TypeFor[Config]())
	if len(missing) > 0 {
		t.Fatalf("exported fields without yaml tags: %s", strings.Join(missing, ", "))
	}
}
