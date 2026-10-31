package pkg

import "testing"

func TestVersionTokenCompatibility(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"47.2.10", "47.2.9", 1}, {"1.2", "1.2.0", 0}, {"1.2.0", "1.2.0-rc.1", 1},
		{"1.2.0-rc.1", "1.2.0-beta.2", 1}, {"1.2.0-alpha", "1.2.0-dev", 1}, {"1.2.0-SNAPSHOT", "1.2.0-snapshot", 0},
		{"26.2.0-beta.10", "26.2.0-beta.2", 1}, {"1.2.0-preview", "1.2.0-beta", 1},
	} {
		got := compareVersionTokens(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("%s vs %s: got %d want %d", tc.a, tc.b, got, tc.want)
		}
		reverse := compareVersionTokens(tc.b, tc.a)
		if reverse != -tc.want {
			t.Errorf("reverse %s vs %s: %d", tc.b, tc.a, reverse)
		}
	}
}
