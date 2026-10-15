// SPDX-License-Identifier: MIT

package utils

import "testing"

func TestNormalizeRemoteURLKeepsOnlyTheFetchableAddress(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			// The shape the panel and this helper exist for: a link copied out
			// of an HTML source or a markdown file. Passed through verbatim,
			// imgix (Unsplash, for one) sees "amp;w" as an unknown parameter,
			// ignores it, and answers with the full-size original.
			"escaped separators",
			"https://images.unsplash.com/photo-1791391279415-fefa6e26f0e2?q=80&amp;w=1336&amp;auto=format&amp;fit=crop",
			"https://images.unsplash.com/photo-1791391279415-fefa6e26f0e2?q=80&w=1336&auto=format&fit=crop",
		},
		{"escaped separator in any case", "https://x.test/a.jpg?q=80&AMP;w=100", "https://x.test/a.jpg?q=80&w=100"},
		{"surrounding whitespace", "  https://x.test/a.jpg  ", "https://x.test/a.jpg"},
		{"already fetchable", "https://x.test/a.jpg?q=80&w=100", "https://x.test/a.jpg?q=80&w=100"},
		// Only the separator itself is rewritten: an escaped ampersand inside
		// a value stays the value the author wrote.
		{"escaped text is not a separator", "https://x.test/a.jpg?title=a%26b", "https://x.test/a.jpg?title=a%26b"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeRemoteURL(tt.raw); got != tt.want {
				t.Errorf("NormalizeRemoteURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
