package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// UpdateValidation replaces only the build/test/lint command lists in an existing
// configuration file, preserving every other key and every comment yaml.v3 can
// retain. It is the supported operation behind `sop init --refresh-validation`,
// so an operator can adopt detected commands without hand-editing the policy.
//
// The existing file must parse cleanly first: an invalid configuration is refused
// rather than partially rewritten. A missing validation block is added in place.
// The existing schema is used exactly; no key is invented.
func UpdateValidation(path string, validation Validation) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := Parse(data); err != nil {
		return fmt.Errorf("config: refusing to edit invalid configuration: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("config: parse yaml: %w", err)
	}
	root := documentRoot(&doc)
	if root == nil || root.Kind != yaml.MappingNode {
		return errors.New("config: configuration root is not a mapping")
	}

	block := mappingValue(root, "validation")
	if block == nil {
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "validation"}
		block = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, key, block)
	}
	if block.Kind != yaml.MappingNode {
		return errors.New("config: validation block is not a mapping")
	}
	setStringList(block, "build", validation.Build)
	setStringList(block, "test", validation.Test)
	setStringList(block, "lint", validation.Lint)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("config: encode yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("config: encode yaml: %w", err)
	}

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc == nil || len(doc.Content) == 0 {
		return nil
	}
	return doc.Content[0]
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func setStringList(mapping *yaml.Node, key string, values []string) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			seq.HeadComment = mapping.Content[i+1].HeadComment
			seq.LineComment = mapping.Content[i+1].LineComment
			mapping.Content[i+1] = seq
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		seq,
	)
}
