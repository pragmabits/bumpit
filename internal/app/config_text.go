package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// configText is the text of a config file as an edit changes it: line by
// line, so that what the edit does not touch stays byte for byte, blank
// lines, comments, indentation and line endings included. The YAML tree only
// locates what the edit changes.
type configText struct {
	// byteOrderMark is the byte order mark the file starts with, if any,
	// kept apart from the first line so that columns count from its text.
	byteOrderMark string
	lines         []textLine
	// root is the root mapping of the file, nil for a file with no content.
	root *yaml.Node
}

// textLine is one line of the file without its line ending, and the ending:
// "\n", "\r\n", or "" for a last line that has none.
type textLine struct {
	text   string
	ending string
}

// parseConfigText reads content as one YAML document whose root is a block
// mapping, the only file an edit can change line by line.
func parseConfigText(content []byte) (*configText, error) {
	documents, err := decodeDocuments(content)
	if err != nil {
		return nil, err
	}
	if len(documents) > 1 {
		return nil, fmt.Errorf("the file holds several YAML documents, which bumpit cannot edit")
	}

	text := &configText{}
	if rest, found := bytes.CutPrefix(content, []byte(byteOrderMark)); found {
		text.byteOrderMark, content = byteOrderMark, rest
	}
	text.lines = splitLines(content)
	if len(documents) == 0 || len(documents[0].Content) == 0 {
		return text, nil
	}
	root := documents[0].Content[0]
	switch {
	case root.Kind == yaml.ScalarNode && root.Tag == "!!null":
		return text, nil
	case root.Kind != yaml.MappingNode:
		return nil, fmt.Errorf("the root of the file is no mapping")
	case root.Style&yaml.FlowStyle != 0:
		return nil, fmt.Errorf("the root of the file is written as a flow mapping {...}, which bumpit cannot edit")
	}
	text.root = root
	return text, nil
}

const byteOrderMark = "\ufeff"

func decodeDocuments(content []byte) ([]*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var documents []*yaml.Node
	for {
		document := &yaml.Node{}
		err := decoder.Decode(document)
		if errors.Is(err, io.EOF) {
			return documents, nil
		}
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
}

func splitLines(content []byte) []textLine {
	var lines []textLine
	for rest := string(content); rest != ""; {
		text, after, found := strings.Cut(rest, "\n")
		line := textLine{text: text}
		if found {
			line.ending = "\n"
			if withoutReturn, found := strings.CutSuffix(text, "\r"); found {
				line.text, line.ending = withoutReturn, "\r\n"
			}
		}
		lines = append(lines, line)
		rest = after
	}
	return lines
}

func (c *configText) bytes() []byte {
	var content strings.Builder
	content.WriteString(c.byteOrderMark)
	for _, line := range c.lines {
		content.WriteString(line.text)
		content.WriteString(line.ending)
	}
	return []byte(content.String())
}

// ending is the line ending a new line takes: the one of the first line that
// has one, "\n" in a file with none.
func (c *configText) ending() string {
	for _, line := range c.lines {
		if line.ending != "" {
			return line.ending
		}
	}
	return "\n"
}

// insert puts texts as new lines before the line at index, len(c.lines) to
// append them, and ends the line before them when it has no ending.
func (c *configText) insert(index int, texts []string) {
	ending := c.ending()
	if index > 0 && c.lines[index-1].ending == "" {
		c.lines[index-1].ending = ending
	}
	inserted := make([]textLine, 0, len(texts))
	for _, text := range texts {
		inserted = append(inserted, textLine{text: text, ending: ending})
	}
	c.lines = slices.Insert(c.lines, index, inserted...)
}

// remove deletes the lines from first to last, both included.
func (c *configText) remove(first, last int) {
	c.lines = slices.Delete(c.lines, first, last+1)
}

// replace puts texts in place of the lines from first to last.
func (c *configText) replace(first, last int, texts []string) {
	c.remove(first, last)
	c.insert(first, texts)
}

// offset is the index in its line of the byte node starts at: YAML counts
// columns in characters, which may take several bytes.
func (c *configText) offset(node *yaml.Node) int {
	text := c.lines[node.Line-1].text
	column := 0
	for index := range text {
		if column == node.Column-1 {
			return index
		}
		column++
	}
	return len(text)
}

// rootColumn is the column the keys of the root mapping start at.
func (c *configText) rootColumn() int {
	if c.root == nil || len(c.root.Content) == 0 {
		return 0
	}
	return c.root.Content[0].Column - 1
}

// rootKey is the key of the root mapping named name and its value, both nil
// when the file has none. Keys are compared without case, as the config is
// read, so two keys that differ only in case are refused.
func (c *configText) rootKey(name string) (*yaml.Node, *yaml.Node, error) {
	if c.root == nil {
		return nil, nil, nil
	}
	return mappingKey(c.root, name)
}

// mappingKey is the key of mapping named name, compared without case, and
// its value.
func mappingKey(mapping *yaml.Node, name string) (*yaml.Node, *yaml.Node, error) {
	var key, value *yaml.Node
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if !strings.EqualFold(mapping.Content[index].Value, name) {
			continue
		}
		if key != nil {
			return nil, nil, fmt.Errorf("%s is written twice (keys are compared without case)", name)
		}
		key, value = mapping.Content[index], mapping.Content[index+1]
	}
	return key, value, nil
}

// blockEnd is the last line of the block that starts at line start with its
// content at column: the lines after it that are indented deeper, blank
// lines and comments after the last of them left out.
func (c *configText) blockEnd(start, column int) int {
	last := start
	for index := start + 1; index < len(c.lines); index++ {
		text := c.lines[index].text
		if !isContent(text) {
			continue
		}
		if indentation(text) <= column {
			break
		}
		last = index
	}
	return last
}

// holdsComment reports whether the lines from first to last, which node
// spans, hold a comment: one that the YAML parser gives a node inside it, or
// one that a line shows, so that neither a quote nor the parser hides it.
func (c *configText) holdsComment(first, last int, node *yaml.Node) bool {
	if nodeHoldsComment(node) {
		return true
	}
	for index := first; index <= last; index++ {
		if commentStart(c.lines[index].text) >= 0 {
			return true
		}
	}
	return false
}

// nodeHoldsComment reports a comment the parser gives node or a node inside
// it, the comment above node left out, since it lies before node's lines.
func nodeHoldsComment(node *yaml.Node) bool {
	if node.LineComment != "" || node.FootComment != "" {
		return true
	}
	for _, child := range node.Content {
		if child.HeadComment != "" || nodeHoldsComment(child) {
			return true
		}
	}
	return false
}

func isContent(text string) bool {
	trimmed := strings.TrimSpace(text)
	return trimmed != "" && !strings.HasPrefix(trimmed, "#")
}

func indentation(text string) int {
	return len(text) - len(strings.TrimLeft(text, " "))
}

// checkEditable refuses what an edit line by line cannot keep right: an
// anchor or an alias, which ties the text to another place of the file, and
// a block scalar, written over lines of its own.
func checkEditable(node *yaml.Node, name string) error {
	switch {
	case node.Anchor != "":
		return fmt.Errorf("%s holds an anchor (&%s), which bumpit cannot edit", name, node.Anchor)
	case node.Kind == yaml.AliasNode:
		return fmt.Errorf("%s holds an alias (*%s), which bumpit cannot edit", name, node.Value)
	case node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0:
		return fmt.Errorf("%s holds a block scalar (| or >), which bumpit cannot edit", name)
	}
	for _, child := range node.Content {
		if err := checkEditable(child, name); err != nil {
			return err
		}
	}
	return nil
}

// commentStart is the index of the # that starts a comment on the line, -1
// when it holds none. A # inside a quoted scalar, or not after a space,
// starts none.
func commentStart(text string) int {
	for index := 0; index < len(text); index++ {
		switch character := text[index]; {
		case (character == '"' || character == '\'') && opensQuote(text[:index]):
			index = closingQuote(text, index) - 1
		case character == '#' && (index == 0 || text[index-1] == ' ' || text[index-1] == '\t'):
			return index
		}
	}
	return -1
}

// opensQuote reports whether a quote after before starts a quoted scalar,
// where a scalar can start: at the start of the line, after an indicator
// and a space, as in "key: '" or "- '", or in a flow collection. A quote
// inside a plain scalar, as in it's or a-'b, starts none.
func opensQuote(before string) bool {
	trimmed := strings.TrimRight(before, " \t")
	switch {
	case trimmed == "":
		return true
	case strings.ContainsAny(trimmed[len(trimmed)-1:], "[{,"):
		return true
	default:
		return trimmed != before && strings.ContainsAny(trimmed[len(trimmed)-1:], ":-?")
	}
}

// closingQuote is the index after the quote that closes the quoted scalar
// starting at start, the end of the line when the line does not close it.
func closingQuote(text string, start int) int {
	quote := text[start]
	for index := start + 1; index < len(text); index++ {
		switch {
		case quote == '"' && text[index] == '\\':
			index++
		case text[index] != quote:
		case quote == '\'' && index+1 < len(text) && text[index+1] == '\'':
			index++
		default:
			return index + 1
		}
	}
	return len(text)
}

// scalarEnd is the index after the scalar that starts at start on the line.
// A plain scalar ends before the # that starts a comment, a quote inside it
// being a character like the others.
func scalarEnd(text string, start int) int {
	if text[start] == '"' || text[start] == '\'' {
		return closingQuote(text, start)
	}
	end := len(text)
	for index := start + 1; index < len(text); index++ {
		if text[index] == '#' && (text[index-1] == ' ' || text[index-1] == '\t') {
			end = index
			break
		}
	}
	return start + len(strings.TrimRight(text[start:end], " \t"))
}

// colonAfterKey is the index of the colon that ends the key starting at
// start on the line.
func colonAfterKey(text string, start int) int {
	index := start
	if text[start] == '"' || text[start] == '\'' {
		index = closingQuote(text, start)
	}
	for ; index < len(text); index++ {
		if text[index] == ':' && (index+1 == len(text) || text[index+1] == ' ' || text[index+1] == '\t') {
			return index
		}
	}
	return len(text)
}

// yamlString is value as a YAML string scalar, double quoted when it holds a
// line break, so that it stays on one line.
func yamlString(value string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if strings.ContainsAny(value, "\r\n") {
		node.Style = yaml.DoubleQuotedStyle
	}
	return node
}

// encodeYAML is the text of node as YAML, its lines without their endings.
func encodeYAML(node *yaml.Node) ([]string, error) {
	var content bytes.Buffer
	encoder := yaml.NewEncoder(&content)
	encoder.SetIndent(2)
	if err := encoder.Encode(node); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSuffix(content.String(), "\n"), "\n"), nil
}
