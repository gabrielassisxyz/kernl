package logging

import "testing"

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDetectColorEnabledHonorsConventions(t *testing.T) {
	tty := func() bool { return true }
	noTTY := func() bool { return false }
	cases := []struct {
		name string
		env  map[string]string
		tty  func() bool
		want bool
	}{
		{"NO_COLOR wins over TTY", map[string]string{"NO_COLOR": "1"}, tty, false},
		{"NO_COLOR wins over FORCE_COLOR", map[string]string{"NO_COLOR": "1", "FORCE_COLOR": "1"}, tty, false},
		{"TERM=dumb disables", map[string]string{"TERM": "dumb"}, tty, false},
		{"CI disables", map[string]string{"CI": "true"}, tty, false},
		{"FORCE_COLOR enables without TTY", map[string]string{"FORCE_COLOR": "1"}, noTTY, true},
		{"TTY enables by default", map[string]string{}, tty, true},
		{"non-TTY disables by default", map[string]string{}, noTTY, false},
	}
	for _, c := range cases {
		if got := detectColorEnabled(envMap(c.env), c.tty); got != c.want {
			t.Errorf("%s: detectColorEnabled = %v, want %v", c.name, got, c.want)
		}
	}
}
