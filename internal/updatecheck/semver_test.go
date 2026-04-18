package updatecheck

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		name           string
		current, latest string
		want           bool
	}{
		{"latest is newer", "0.5.2", "0.5.3", true},
		{"latest is newer with v prefix", "v0.5.2", "v0.5.3", true},
		{"mixed prefixes", "0.5.2", "v0.6.0", true},
		{"latest is older", "0.6.0", "0.5.9", false},
		{"equal", "0.5.2", "0.5.2", false},
		{"latest is major bump", "0.5.2", "1.0.0", true},
		{"prerelease less than release", "1.0.0-rc.1", "1.0.0", true},
		{"release greater than prerelease", "1.0.0", "1.0.0-rc.1", false},
		{"invalid current returns false", "not-a-version", "1.0.0", false},
		{"invalid latest returns false", "1.0.0", "not-a-version", false},
		{"empty current returns false", "", "1.0.0", false},
		{"empty latest returns false", "1.0.0", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsNewer(tc.current, tc.latest)
			if got != tc.want {
				t.Fatalf("IsNewer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0.5.2", "v0.5.2"},
		{"v0.5.2", "v0.5.2"},
		{"  v1.0.0  ", "v1.0.0"},
		{"", ""},
		{"garbage", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := Normalize(tc.in)
			if got != tc.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
