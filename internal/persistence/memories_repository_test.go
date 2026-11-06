package persistence

import (
	"testing"
)

func TestConditionBuilderForBM25(t *testing.T) {
	testCases := []struct {
		name        string
		searchQuery string
		expected    string
		expectError bool
	}{
		{
			name:        "normal two word query",
			searchQuery: "hello world",
			expected:    `"hello"OR"world"`,
		},
		{
			name:        "normal three word query",
			searchQuery: "mem never calls",
			expected:    `"mem"OR"never"OR"calls"`,
		},
		{
			name:        "single word query",
			searchQuery: "hello",
			expected:    `"hello"`,
		},
		{
			name:        "empty string",
			searchQuery: "",
			expectError: true,
		},
		{
			name:        "single space",
			searchQuery: " ",
			expectError: true,
		},
		{
			name:        "multiple spaces only",
			searchQuery: "   ",
			expectError: true,
		},
		{
			name:        "tab only",
			searchQuery: "\t",
			expectError: true,
		},
		{
			name:        "newlines only",
			searchQuery: "\n\n\n",
			expectError: true,
		},
		{
			name:        "quote only",
			searchQuery: "\"",
			expectError: true,
		},
		{
			name:        "mixed whitespace only",
			searchQuery: "   \t\n  ",
			expectError: true,
		},
		{
			name:        "leading space",
			searchQuery: " hello",
			expected:    `"hello"`,
		},
		{
			name:        "trailing space",
			searchQuery: "hello ",
			expected:    `"hello"`,
		},
		{
			name:        "leading and trailing spaces",
			searchQuery: "  hello  ",
			expected:    `"hello"`,
		},
		{
			name:        "two consecutive spaces between words",
			searchQuery: "hello  world",
			expected:    `"hello"OR"world"`,
		},
		{
			name:        "three consecutive spaces between words",
			searchQuery: "hello   world",
			expected:    `"hello"OR"world"`,
		},
		{
			name:        "five consecutive spaces between words",
			searchQuery: "hello     world",
			expected:    `"hello"OR"world"`,
		},
		{
			name:        "seven consecutive spaces between words",
			searchQuery: "hello       world",
			expected:    `"hello"OR"world"`,
		},
		{
			name:        "irregular spacing all over a sentence",
			searchQuery: "  hello  world  foo  ",
			expected:    `"hello"OR"world"OR"foo"`,
		},
		{
			name:        "embedded quote with no surrounding space",
			searchQuery: `say"hi`,
			expected:    `"say"OR"hi"`,
		},
		{
			name:        "doubled quote with no surrounding space",
			searchQuery: `a"b c""d`,
			expected:    `"a"OR"b"OR"c"OR"d"`,
		},
		{
			name:        "tabs and newlines separating words",
			searchQuery: "line\tone\nline\ttwo",
			expected:    `"line"OR"one"OR"line"OR"two"`,
		},
	}

	for _, tCase := range testCases {
		t.Run(tCase.name, func(t *testing.T) {
			result, err := conditionBuilderForBM25(tCase.searchQuery)

			if tCase.expectError {
				if err == nil {
					t.Logf("expected an error for searchQuery %q, got none (result: %v)", tCase.searchQuery, result)
					t.FailNow()
				}
				return
			}

			if err != nil {
				t.Logf("unexpected error for searchQuery %q: %+v", tCase.searchQuery, err)
				t.FailNow()
			}

			if result == nil {
				t.Logf("expected a result for searchQuery %q, got nil", tCase.searchQuery)
				t.FailNow()
			}

			if *result != tCase.expected {
				t.Logf("incorrect result for searchQuery %q! expected %q, got %q", tCase.searchQuery, tCase.expected, *result)
				t.FailNow()
			}
		})
	}
}
