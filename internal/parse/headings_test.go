package parse

import (
	"reflect"
	"testing"
)

func TestHeadings(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []Heading
	}{
		{
			name: "levels 1 through 6",
			body: "# H1\n## H2\n### H3\n#### H4\n##### H5\n###### H6\n",
			want: []Heading{
				{Text: "H1", Level: 1, Line: 1},
				{Text: "H2", Level: 2, Line: 2},
				{Text: "H3", Level: 3, Line: 3},
				{Text: "H4", Level: 4, Line: 4},
				{Text: "H5", Level: 5, Line: 5},
				{Text: "H6", Level: 6, Line: 6},
			},
		},
		{
			name: "more than 6 hashes is not a heading",
			body: "####### H7\n",
			want: nil,
		},
		{
			name: "no space after hashes is not a heading",
			body: "#H1\n##H2\n",
			want: nil,
		},
		{
			name: "indentation up to 3 spaces is valid",
			body: " # One space\n  ## Two spaces\n   ### Three spaces\n    #### Four spaces (indented code)\n",
			want: []Heading{
				{Text: "One space", Level: 1, Line: 1},
				{Text: "Two spaces", Level: 2, Line: 2},
				{Text: "Three spaces", Level: 3, Line: 3},
			},
		},
		{
			name: "closing hashes stripped when preceded by space",
			body: "## Heading ##\n## Heading ###\n## Heading #\n## C#\n",
			want: []Heading{
				{Text: "Heading", Level: 2, Line: 1},
				{Text: "Heading", Level: 2, Line: 2},
				{Text: "Heading", Level: 2, Line: 3},
				{Text: "C#", Level: 2, Line: 4},
			},
		},
		{
			name: "empty heading text",
			body: "##\n##   \n## ###\n",
			want: []Heading{
				{Text: "", Level: 2, Line: 1},
				{Text: "", Level: 2, Line: 2},
				{Text: "", Level: 2, Line: 3},
			},
		},
		{
			name: "headings inside fenced code blocks are ignored",
			body: "## Before\n\n```python\n# comment inside code\n## also in code\n```\n\n## After\n\n~~~sh\n# sh comment\n~~~\n\n### End\n",
			want: []Heading{
				{Text: "Before", Level: 2, Line: 1},
				{Text: "After", Level: 2, Line: 8},
				{Text: "End", Level: 3, Line: 14},
			},
		},
		{
			name: "CRLF line endings",
			body: "## First\r\n\r\n## Second\r\n",
			want: []Heading{
				{Text: "First", Level: 2, Line: 1},
				{Text: "Second", Level: 2, Line: 3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Headings(tt.body)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Headings() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNormalizeHeading(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Context", "context"},
		{"  Decision  ", "decision"},
		{"Consequences:", "consequences"},
		{"  Context and Problem Statement:  ", "context and problem statement"},
		{"ステータス", "ステータス"},
		{"日付:", "日付"},
		{"Decision Outcome", "decision outcome"},
	}

	for _, tt := range tests {
		if got := NormalizeHeading(tt.input); got != tt.want {
			t.Errorf("NormalizeHeading(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
