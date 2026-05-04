package proxy

import "testing"

func TestLogEnabled(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"no", false},
		{"off", false},
		{"random", false},
		{"1", true},
		{"true", true},
		{"True", true},
		{"YES", true},
		{"on", true},
	}
	for _, c := range cases {
		if got := logEnabled(c.in); got != c.want {
			t.Errorf("logEnabled(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
