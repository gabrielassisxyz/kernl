package tags

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/ids"
	"github.com/gabrielassisxyz/kernl/internal/graph/tagname"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// tagNodes creates one node per tag and returns them keyed by tag name.
func tagNodes(t *testing.T, g *graph.Graph, tagNames ...string) map[string]string {
	t.Helper()
	ctx := context.Background()
	author := Author{Name: "test"}
	byTag := make(map[string]string, len(tagNames))

	for _, name := range tagNames {
		nodeID := ids.New()
		err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			if _, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'test', ?)`, nodeID, name); err != nil {
				return err
			}
			return Add(ctx, tx, nodeID, name, author)
		})
		if err != nil {
			t.Fatalf("setup %q: %v", name, err)
		}
		byTag[name] = nodeID
	}
	return byTag
}

func nodesUnder(t *testing.T, g *graph.Graph, name string) []string {
	t.Helper()
	ctx := context.Background()
	var got []string
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = NodesUnder(ctx, tx, name)
		return err
	})
	if err != nil {
		t.Fatalf("NodesUnder(%q): %v", name, err)
	}
	sort.Strings(got)
	return got
}

func TestNodesUnderIncludesDescendants(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	byTag := tagNodes(t, g, "homelab", "homelab/nas", "homelab/nas/zfs", "carreira")

	want := []string{byTag["homelab"], byTag["homelab/nas"], byTag["homelab/nas/zfs"]}
	sort.Strings(want)

	if got := nodesUnder(t, g, "homelab"); !reflect.DeepEqual(got, want) {
		t.Errorf("NodesUnder(homelab) = %v, want %v", got, want)
	}
}

// A parent tag must match by segment, not by string prefix: `home` is not an
// ancestor of `homelab`.
func TestNodesUnderIsNotASubstringMatch(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	tagNodes(t, g, "homelab", "homelab/nas")

	if got := nodesUnder(t, g, "home"); len(got) != 0 {
		t.Errorf("NodesUnder(home) = %v, want none", got)
	}
}

// A tag whose name contains LIKE metacharacters must be matched literally.
func TestNodesUnderEscapesLikeWildcards(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	byTag := tagNodes(t, g, `50%_off`, `50%_off/black-friday`, "50-anything-off")

	want := []string{byTag[`50%_off`], byTag[`50%_off/black-friday`]}
	sort.Strings(want)

	if got := nodesUnder(t, g, `50%_off`); !reflect.DeepEqual(got, want) {
		t.Errorf("NodesUnder(50%%_off) = %v, want %v", got, want)
	}
}

func TestNodesUnderNormalizesTheQuery(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	byTag := tagNodes(t, g, "Homelab/NAS")

	if got := nodesUnder(t, g, " HomeLab "); !reflect.DeepEqual(got, []string{byTag["Homelab/NAS"]}) {
		t.Errorf("NodesUnder( HomeLab ) = %v, want the homelab/nas node", got)
	}
}

func TestNodesUnderRejectsMalformedName(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		_, err := NodesUnder(ctx, tx, "foo//bar")
		return err
	})
	if !errors.Is(err, tagname.ErrInvalid) {
		t.Errorf("NodesUnder(foo//bar) error = %v, want ErrInvalid", err)
	}
}

func TestNodesUnderUnknownTagIsEmpty(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	tagNodes(t, g, "homelab")

	if got := nodesUnder(t, g, "nothing"); len(got) != 0 {
		t.Errorf("NodesUnder(nothing) = %v, want none", got)
	}
}

func TestTreeNestsByConvention(t *testing.T) {
	got := Tree([]string{"homelab/nas", "carreira", "homelab", "homelab/nas/zfs"})

	if len(got) != 2 {
		t.Fatalf("want 2 roots, got %d: %+v", len(got), got)
	}
	if got[0].Name != "carreira" || len(got[0].Children) != 0 {
		t.Errorf("first root = %+v, want a childless carreira", got[0])
	}

	homelab := got[1]
	if homelab.Name != "homelab" || homelab.Segment != "homelab" || len(homelab.Children) != 1 {
		t.Fatalf("second root = %+v, want homelab with one child", homelab)
	}
	nas := homelab.Children[0]
	if nas.Name != "homelab/nas" || nas.Segment != "nas" || len(nas.Children) != 1 {
		t.Fatalf("homelab child = %+v, want homelab/nas with one child", nas)
	}
	if zfs := nas.Children[0]; zfs.Name != "homelab/nas/zfs" || zfs.Segment != "zfs" {
		t.Errorf("nas child = %+v, want homelab/nas/zfs", zfs)
	}
}

// A tag that only exists as a child still needs a parent branch to hang from.
func TestTreeSynthesisesMissingParents(t *testing.T) {
	got := Tree([]string{"homelab/nas/zfs"})

	if len(got) != 1 || got[0].Name != "homelab" {
		t.Fatalf("want a synthesised homelab root, got %+v", got)
	}
	nas := got[0].Children[0]
	if nas.Name != "homelab/nas" || nas.Children[0].Name != "homelab/nas/zfs" {
		t.Errorf("want homelab > homelab/nas > homelab/nas/zfs, got %+v", got[0])
	}
}
