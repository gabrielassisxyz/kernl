package tagname_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph/tagname"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"lowercases", "Homelab", "homelab"},
		{"trims", "  homelab  ", "homelab"},
		{"keeps nesting", "homelab/nas/zfs", "homelab/nas/zfs"},
		{"lowercases every segment", "Homelab/NAS", "homelab/nas"},
		{"trims each segment", "homelab / nas", "homelab/nas"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tagname.Normalize(tc.in)
			if err != nil {
				t.Fatalf("Normalize(%q): unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeRejects(t *testing.T) {
	for _, in := range []string{"", "   ", "/foo", "foo/", "foo//bar", "/", "foo/ /bar"} {
		t.Run(in, func(t *testing.T) {
			if _, err := tagname.Normalize(in); !errors.Is(err, tagname.ErrInvalid) {
				t.Errorf("Normalize(%q) error = %v, want ErrInvalid", in, err)
			}
		})
	}
}

func TestNormalizeAllDeduplicates(t *testing.T) {
	got, err := tagname.NormalizeAll([]string{"Homelab", "homelab", " HOMELAB ", "homelab/nas"})
	if err != nil {
		t.Fatalf("NormalizeAll: unexpected error: %v", err)
	}
	want := []string{"homelab", "homelab/nas"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeAll = %v, want %v", got, want)
	}
}

func TestNormalizeAllFailsOnFirstInvalid(t *testing.T) {
	if _, err := tagname.NormalizeAll([]string{"homelab", "foo//bar"}); !errors.Is(err, tagname.ErrInvalid) {
		t.Errorf("NormalizeAll error = %v, want ErrInvalid", err)
	}
}

func TestIsUnder(t *testing.T) {
	cases := []struct {
		name     string
		ancestor string
		want     bool
	}{
		{"homelab", "homelab", true},
		{"homelab/nas", "homelab", true},
		{"homelab/nas/zfs", "homelab", true},
		{"homelab", "home", false}, // prefix of a segment, not a parent
		{"homelabx", "homelab", false},
		{"nas/homelab", "homelab", false},
	}
	for _, tc := range cases {
		if got := tagname.IsUnder(tc.name, tc.ancestor); got != tc.want {
			t.Errorf("IsUnder(%q, %q) = %v, want %v", tc.name, tc.ancestor, got, tc.want)
		}
	}
}
