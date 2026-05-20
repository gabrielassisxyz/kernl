package nodes

import (
	"time"
)

// NodeSpec is what the chokepoint accepts.
type NodeSpec interface {
	Meta() *Meta
	NodeAttrs() []byte    // JSON attrs payload
	NodeTags() []string
	FTSFields() FTSFields // returns searchable text
}

// DiffableNode opt-in interface.
type DiffableNode interface {
	NodeSpec
	DiffBody(prev NodeSpec) []byte // diff JSON or nil
}

// FTSFields groups what goes into the FTS virtual table.
type FTSFields struct {
	Title string
	Body  string // e.g. description, concatenated fields
	Tags  string
}

// Meta is embedded in every concrete node type.
type Meta struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (m Meta) NodeID() string { return m.ID }

// Author models who performed a mutation.
type Author struct {
	Name string // e.g. "gabriel" or agent string
}

func (a Author) Valid() bool  { return a.Name != "" }
func (a Author) String() string { return a.Name }

// AuthorAgent returns an agent-prefixed author (e.g. AuthorAgent("kimi") -> "agent:kimi").
func AuthorAgent(name string) Author { return Author{Name: "agent:" + name} }
