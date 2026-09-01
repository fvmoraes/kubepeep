package resources

import "testing"

func TestParseSearch(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected SearchQuery
	}{
		{
			name:  "single term",
			input: "payment",
			expected: SearchQuery{
				Include: []string{"payment"},
			},
		},
		{
			name:  "multiple terms",
			input: "payment api gateway",
			expected: SearchQuery{
				Include: []string{"payment", "api", "gateway"},
			},
		},
		{
			name:  "exclude term",
			input: "payment -failed",
			expected: SearchQuery{
				Include: []string{"payment"},
				Exclude: []string{"failed"},
			},
		},
		{
			name:  "exclude with bang",
			input: "payment !failed",
			expected: SearchQuery{
				Include: []string{"payment"},
				Exclude: []string{"failed"},
			},
		},
		{
			name:  "phrase",
			input: `"exact phrase"`,
			expected: SearchQuery{
				IncludePhrases: []string{"exact phrase"},
			},
		},
		{
			name:  "exclude phrase",
			input: `payment -"exact phrase"`,
			expected: SearchQuery{
				Include:        []string{"payment"},
				ExcludePhrases: []string{"exact phrase"},
			},
		},
		{
			name:  "mixed",
			input: `payment api -failed !"out of memory"`,
			expected: SearchQuery{
				Include:        []string{"payment", "api"},
				Exclude:        []string{"failed"},
				ExcludePhrases: []string{"out of memory"},
			},
		},
		{
			name:     "empty",
			input:    "",
			expected: SearchQuery{},
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: SearchQuery{},
		},
		{
			name:  "quoted with escaped quote",
			input: `"say \"hello\""`,
			expected: SearchQuery{
				IncludePhrases: []string{`say "hello"`},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ParseSearch(test.input)
			if !queriesEqual(got, test.expected) {
				t.Fatalf("ParseSearch(%q) = %+v, want %+v", test.input, got, test.expected)
			}
		})
	}
}

func TestSearchQueryMatches(t *testing.T) {
	tests := []struct {
		name     string
		query    SearchQuery
		fields   []string
		expected bool
	}{
		{name: "empty matches", query: SearchQuery{}, fields: []string{"anything"}, expected: true},
		{name: "include matches", query: SearchQuery{Include: []string{"api"}}, fields: []string{"payment-api"}, expected: true},
		{name: "include misses", query: SearchQuery{Include: []string{"api"}}, fields: []string{"payment-ui"}, expected: false},
		{name: "exclude matches", query: SearchQuery{Include: []string{"payment"}, Exclude: []string{"failed"}}, fields: []string{"payment", "running"}, expected: true},
		{name: "exclude blocks", query: SearchQuery{Include: []string{"payment"}, Exclude: []string{"failed"}}, fields: []string{"payment", "failed"}, expected: false},
		{name: "phrase matches", query: SearchQuery{IncludePhrases: []string{"out of memory"}}, fields: []string{"pod crashed with out of memory"}, expected: true},
		{name: "phrase misses", query: SearchQuery{IncludePhrases: []string{"out of memory"}}, fields: []string{"out memory"}, expected: false},
		{name: "exclude phrase", query: SearchQuery{Include: []string{"payment"}, ExcludePhrases: []string{"out of memory"}}, fields: []string{"payment", "out of memory"}, expected: false},
		{name: "case folding", query: SearchQuery{Include: []string{"API"}}, fields: []string{"api"}, expected: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.query.Matches(test.fields...)
			if got != test.expected {
				t.Fatalf("Matches(%v) = %v, want %v", test.fields, got, test.expected)
			}
		})
	}
}

func queriesEqual(a, b SearchQuery) bool {
	return slicesEqual(a.Include, b.Include) &&
		slicesEqual(a.Exclude, b.Exclude) &&
		slicesEqual(a.IncludePhrases, b.IncludePhrases) &&
		slicesEqual(a.ExcludePhrases, b.ExcludePhrases)
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
