package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Update assigns Value at the dotted YAML location described by Path.
// Intermediate mappings are created when they do not exist yet.
type Update struct {
	Path  []string
	Value any
}

// Apply writes updates into the YAML document at path.
//
// The document is round-tripped through yaml.Node rather than re-marshalled from
// Config, so every comment, key order, and unknown field the user wrote by hand
// survives the write. Callers must only pass whitelisted, already-validated
// paths: this function is a typed field writer, not a general YAML editor.
//
// The write is atomic (temp file + rename) so a crash mid-write cannot leave the
// only copy of the user's config truncated.
func Apply(path string, updates []Update) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: reading config %s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: parsing config %s: %w", path, err)
	}

	root, err := documentRoot(&doc)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: config %s: %w", path, err)
	}

	for _, update := range updates {
		if err := setPath(root, update.Path, update.Value); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: setting %v in %s: %w", update.Path, path, err)
		}
	}

	encoded, err := encodeDocument(&doc)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: encoding config %s: %w", path, err)
	}

	// A config that no longer loads is worse than a rejected write, so prove the
	// result parses back into Config before it replaces the original.
	var probe Config
	if err := yaml.Unmarshal(encoded, &probe); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: refusing to write %s: result does not parse: %w", path, err)
	}

	return writeAtomic(path, encoded)
}

// documentRoot returns the top-level mapping of doc, initializing it when the
// file was empty.
func documentRoot(doc *yaml.Node) (*yaml.Node, error) {
	if doc.Kind == 0 || len(doc.Content) == 0 {
		root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		doc.Kind = yaml.DocumentNode
		doc.Content = []*yaml.Node{root}
		return root, nil
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("top level is not a mapping")
	}
	return root, nil
}

// setPath walks (and grows) the mapping tree to assign value at path.
func setPath(mapping *yaml.Node, path []string, value any) error {
	if len(path) == 0 {
		return fmt.Errorf("empty path")
	}

	key := path[0]
	valueNode := mappingValue(mapping, key)

	if len(path) == 1 {
		if valueNode == nil {
			valueNode = appendKey(mapping, key)
		}
		return assign(valueNode, value)
	}

	if valueNode == nil {
		valueNode = appendKey(mapping, key)
		valueNode.Kind = yaml.MappingNode
		valueNode.Tag = "!!map"
	}
	if valueNode.Kind != yaml.MappingNode {
		return fmt.Errorf("%q is not a mapping", key)
	}
	return setPath(valueNode, path[1:], value)
}

// mappingValue returns the value node for key, or nil when the key is absent.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// appendKey adds an empty entry for key and returns its value node.
func appendKey(mapping *yaml.Node, key string) *yaml.Node {
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
	return valueNode
}

// assign overwrites the node's content with value while keeping the comments the
// user attached to it.
func assign(node *yaml.Node, value any) error {
	var encoded yaml.Node
	if err := encoded.Encode(value); err != nil {
		return err
	}

	head, line, foot := node.HeadComment, node.LineComment, node.FootComment
	style := node.Style
	*node = encoded
	node.HeadComment, node.LineComment, node.FootComment = head, line, foot

	// Preserve a quoting style the user chose (e.g. a quoted path), but never
	// force a style onto a value that cannot carry one.
	if node.Kind == yaml.ScalarNode && encoded.Tag == "!!str" {
		node.Style = style
	}
	return nil
}

func encodeDocument(doc *yaml.Node) ([]byte, error) {
	var buf []byte
	writer := &byteWriter{buf: &buf}

	encoder := yaml.NewEncoder(writer)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

type byteWriter struct{ buf *[]byte }

func (w *byteWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

// writeAtomic replaces path with data, keeping the original file mode.
func writeAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: creating temp config in %s: %w", dir, err)
	}
	tempPath := temp.Name()
	// Cleans up the temp file on every failure path. After a successful rename
	// there is nothing left at tempPath, and the failure to remove it is not worth
	// failing an otherwise complete write.
	defer func() { _ = os.Remove(tempPath) }()

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("KERNL DISPATCH FAILURE: writing temp config %s: %w", tempPath, err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("KERNL DISPATCH FAILURE: syncing temp config %s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: closing temp config %s: %w", tempPath, err)
	}
	if err := os.Chmod(tempPath, mode); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: setting mode on %s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: replacing config %s: %w", path, err)
	}
	return nil
}
