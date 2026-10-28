package ui

import "testing"

func TestSanitize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
	}{
		{"ansi, bell and tab", "\x1b[31mred\x1b[0m\x07bell\tx", "redbellx"},
		{"screen clear", "evil\x1b[2Jname", "evilname"},
		{"title escape", "a\x1b]0;owned\x07b", "ab"},
		{"newlines and carriage returns", "one\ntwo\r\nthree", "onetwothree"},
		{"delete and c1 controls", "x\x7fy\u009bz", "xyz"},
		{"bidi embeddings and overrides", "a\u202Ab\u202Bc\u202Cd\u202De\u202Ef", "abcdef"},
		{"bidi isolates", "a\u2066b\u2067c\u2068d\u2069e", "abcde"},
		{"bidi marks", "a\u200Eb\u200Fc", "abc"},
		{"zero-width characters", "a\u200Bb\u200Cc\u200Dd\uFEFFe", "abcde"},
		{"line and paragraph separators", "a\u2028b\u2029c", "abc"},
		{"soft hyphen and word joiners", "a\u00ADb\u2060c\u2063d", "abcd"},
		{"arabic letter mark and mongolian vowel separator", "j\u061Ck\u180El", "jkl"},
		{"interlinear annotation", "p\uFFF9q\uFFFBr", "pqr"},
		{"tag characters", "a\U000E0001\U000E0041b", "ab"},
		{"variation selector kept", "\u2764\uFE0F", "\u2764\uFE0F"},
		{"unicode and spaces kept", "feature/vat café 汉字 a b", "feature/vat café 汉字 a b"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := sanitize(tt.in); got != tt.want {
				t.Errorf("sanitize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
