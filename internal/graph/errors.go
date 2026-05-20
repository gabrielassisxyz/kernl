package graph

import "errors"

var (
	ErrNotFound       = errors.New("graph: not found")
	ErrFTSQuerySyntax = errors.New("graph: fts query syntax")
	ErrSchemaLocked   = errors.New("graph: schema locked")
	ErrAuthorRequired = errors.New("graph: author required")
	ErrEmptyTag       = errors.New("graph: empty tag")
)
