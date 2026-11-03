package app

import (
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// errConfigKeyNotSet is returned for a key the config file does not set.
var errConfigKeyNotSet = errors.New("the key is not set")

// configKeyValue is the value the config text gives the root key, compared
// without case, empty for a key written with no value, and false when the
// file leaves the key out.
func configKeyValue(content []byte, key string) (string, bool, error) {
	text, err := parseConfigText(content)
	if err != nil {
		return "", false, err
	}
	_, value, err := text.rootKey(key)
	if err != nil || value == nil {
		return "", false, err
	}
	if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		return "", true, nil
	}
	if value.Kind != yaml.ScalarNode {
		return "", false, fmt.Errorf("%s is no single value", key)
	}
	return value.Value, true, nil
}

// setConfigKeyText gives the root key the YAML text value, in place of the
// value it has, keeping the comment after it, or as a new key at the end of
// the file.
func setConfigKeyText(content []byte, key, value string) ([]byte, error) {
	text, keyNode, valueNode, err := parseConfigKey(content, key)
	if err != nil {
		return nil, err
	}
	if keyNode == nil {
		text.insert(len(text.lines), []string{strings.Repeat(" ", text.rootColumn()) + key + ": " + value})
		return text.bytes(), nil
	}

	line := &text.lines[keyNode.Line-1]
	if valueNode.Tag == "!!null" && valueNode.Value == "" {
		colon := colonAfterKey(line.text, text.offset(keyNode))
		line.text = line.text[:colon+1] + " " + value + line.text[colon+1:]
		return text.bytes(), nil
	}
	start := text.offset(valueNode)
	line.text = line.text[:start] + value + line.text[scalarEnd(line.text, start):]
	return text.bytes(), nil
}

// unsetConfigKeyText removes the root key and its value, with the comments
// on their lines, and returns errConfigKeyNotSet when the file has no such
// key.
func unsetConfigKeyText(content []byte, key string) ([]byte, error) {
	text, keyNode, _, err := parseConfigKey(content, key)
	if err != nil {
		return nil, err
	}
	if keyNode == nil {
		return nil, errConfigKeyNotSet
	}
	text.remove(keyNode.Line-1, keyNode.Line-1)
	return text.bytes(), nil
}

// parseConfigKey locates the root key, nil when the file has none, and
// refuses a value an edit of its line cannot change: one that is no single
// value, or that its line does not hold whole.
func parseConfigKey(content []byte, key string) (*configText, *yaml.Node, *yaml.Node, error) {
	text, err := parseConfigText(content)
	if err != nil {
		return nil, nil, nil, err
	}
	keyNode, valueNode, err := text.rootKey(key)
	if err != nil || keyNode == nil {
		return text, nil, nil, err
	}
	if err := checkEditable(keyNode, key); err != nil {
		return nil, nil, nil, err
	}
	if err := checkEditable(valueNode, key); err != nil {
		return nil, nil, nil, err
	}

	keyLine := keyNode.Line - 1
	switch {
	case valueNode.Kind != yaml.ScalarNode:
		return nil, nil, nil, fmt.Errorf("%s is no single value", key)
	case valueNode.Line-1 != keyLine && valueNode.Value != "", text.blockEnd(keyLine, keyNode.Column-1) != keyLine:
		return nil, nil, nil, fmt.Errorf("%s is written over several lines, which bumpit cannot edit", key)
	}
	return text, keyNode, valueNode, nil
}
