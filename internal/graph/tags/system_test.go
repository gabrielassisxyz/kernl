package tags_test

import (
	"errors"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
)

func TestIsSystem(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"sys/pending", true},
		{"sys/anything/nested", true},
		// The check runs before normalisation, so it must not be dodgeable by
		// the casing or padding a hand-authored tag arrives with.
		{"SYS/pending", true},
		{"  sys/pending  ", true},
		// A subject that merely starts with the letters "sys" is a user tag.
		{"sysadmin", false},
		{"system", false},
		{"homelab/sys", false},
		{"telos", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := tags.IsSystem(tc.name); got != tc.want {
			t.Errorf("IsSystem(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRejectSystem(t *testing.T) {
	if err := tags.RejectSystem([]string{"homelab", "homelab/nas", "telos"}); err != nil {
		t.Errorf("RejectSystem(user tags) = %v, want nil", err)
	}
	err := tags.RejectSystem([]string{"homelab", "sys/pending"})
	if !errors.Is(err, tags.ErrSystemTag) {
		t.Errorf("RejectSystem(with system tag) = %v, want ErrSystemTag", err)
	}
	if err := tags.RejectSystem(nil); err != nil {
		t.Errorf("RejectSystem(nil) = %v, want nil", err)
	}
}
