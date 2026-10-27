package remote

import "testing"

func TestParseHost(t *testing.T) {
	good := map[string]string{"192.168.150.7": "192.168.150.7", "  10.0.0.1 ": "10.0.0.1"}
	for in, want := range good {
		if got, err := ParseHost(in); err != nil || got != want {
			t.Errorf("ParseHost(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "host", `\\10.0.0.1\C$`, "10.0.0.1/c", "::1", "300.1.1.1", "10.0.0"} {
		if _, err := ParseHost(in); err == nil {
			t.Errorf("ParseHost(%q): ожидалась ошибка", in)
		}
	}
}

func TestSharePath(t *testing.T) {
	if got := SharePath("10.0.0.1", "D"); got != `\\10.0.0.1\D$` {
		t.Errorf("SharePath = %q", got)
	}
}
