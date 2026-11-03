package app

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// addModuleText adds the declaration to the modules of the config text: as
// a new item after the last one, in its indentation, or under a modules key
// that lists none, or under a new modules key at the end of the file.
func addModuleText(content []byte, declaration ModuleDeclaration) ([]byte, error) {
	text, section, err := parseModules(content)
	if err != nil {
		return nil, err
	}
	if _, found, err := section.find(declaration.Directory); err != nil || found {
		if err == nil {
			err = fmt.Errorf("%s is declared already: change it with bumpit modules set", declaration.Directory)
		}
		return nil, err
	}

	switch {
	case section.key == nil:
		return text.appendModules(declaration)
	case len(section.value.Content) == 0:
		return text.firstModule(section, declaration)
	}
	last, err := text.itemOf(section.value.Content[len(section.value.Content)-1])
	if err != nil {
		return nil, err
	}
	first, err := text.itemOf(section.value.Content[0])
	if err != nil {
		return nil, err
	}
	lines, err := moduleLines(declaration, first.dashColumn, first.dashPrefix)
	if err != nil {
		return nil, err
	}
	text.insert(last.last+1, lines)
	return text.bytes(), nil
}

// setModuleText writes the declaration in place of the one of its directory.
// It refuses a declaration that holds a comment, which the new text would
// lose.
func setModuleText(content []byte, declaration ModuleDeclaration) ([]byte, error) {
	text, _, item, err := parseDeclaredModule(content, declaration.Directory)
	if err != nil {
		return nil, err
	}
	if text.holdsComment(item.first, item.last, item.node) {
		return nil, fmt.Errorf("the declaration of %s holds a comment, which bumpit modules set would lose: edit it by hand", declaration.Directory)
	}
	lines, err := moduleLines(declaration, item.dashColumn, item.dashPrefix)
	if err != nil {
		return nil, err
	}
	text.replace(item.first, item.last, lines)
	return text.bytes(), nil
}

// removeModuleText removes the declaration of directory, the comments inside
// it included. Removing the last one leaves modules: [], which still
// declares that the repository has no other module.
func removeModuleText(content []byte, directory string) ([]byte, error) {
	text, section, item, err := parseDeclaredModule(content, directory)
	if err != nil {
		return nil, err
	}
	text.remove(item.first, item.last)
	if len(section.value.Content) == 1 {
		line := &text.lines[section.key.Line-1]
		colon := colonAfterKey(line.text, text.offset(section.key))
		line.text = line.text[:colon+1] + " []" + line.text[colon+1:]
	}
	return text.bytes(), nil
}

// modulesSection is the modules key of the root mapping and its value: a
// block list, an empty flow list or no value. Both are nil when the file has
// no modules key.
type modulesSection struct {
	key   *yaml.Node
	value *yaml.Node
}

func parseModules(content []byte) (*configText, modulesSection, error) {
	text, err := parseConfigText(content)
	if err != nil {
		return nil, modulesSection{}, err
	}
	key, value, err := text.rootKey("modules")
	if err != nil || key == nil {
		return text, modulesSection{}, err
	}
	if err := checkModulesValue(key, value); err != nil {
		return nil, modulesSection{}, err
	}
	return text, modulesSection{key: key, value: value}, nil
}

// checkModulesValue refuses a modules key an edit cannot change line by
// line, and a value that is no list.
func checkModulesValue(key, value *yaml.Node) error {
	if err := checkEditable(key, "modules"); err != nil {
		return err
	}
	if err := checkEditable(value, "modules"); err != nil {
		return err
	}
	return checkModulesShape(key, value)
}

// checkModulesShape refuses a modules value that is no list, a flow list
// with items, and an empty value written on a line of its own, which the
// first item would leave behind.
func checkModulesShape(key, value *yaml.Node) error {
	switch {
	case value.Kind == yaml.SequenceNode && len(value.Content) > 0:
		if value.Style&yaml.FlowStyle != 0 {
			return fmt.Errorf("modules is written as a list in [...], which bumpit cannot edit: write each module as an item of a block list under the modules key")
		}
		return nil
	case !isEmptyModules(value):
		return fmt.Errorf("modules is no list")
	case isWritten(value) && value.Line != key.Line:
		return fmt.Errorf("the empty value of modules is written on a line of its own, which bumpit cannot edit: write it on the line of the modules key")
	default:
		return nil
	}
}

// isEmptyModules reports an empty list or a null, a modules key that
// declares no module.
func isEmptyModules(value *yaml.Node) bool {
	return value.Kind == yaml.SequenceNode || value.Kind == yaml.ScalarNode && value.Tag == "!!null"
}

func parseDeclaredModule(content []byte, directory string) (*configText, modulesSection, moduleItem, error) {
	text, section, err := parseModules(content)
	if err != nil {
		return nil, modulesSection{}, moduleItem{}, err
	}
	index, found, err := section.find(directory)
	if err == nil && !found {
		err = fmt.Errorf("%s is not declared: declare it with bumpit modules add", directory)
	}
	if err != nil {
		return nil, modulesSection{}, moduleItem{}, err
	}
	item, err := text.itemOf(section.value.Content[index])
	return text, section, item, err
}

// find is the index of the item that declares directory.
func (s modulesSection) find(directory string) (int, bool, error) {
	if s.key == nil {
		return 0, false, nil
	}
	for index, item := range s.value.Content {
		if item.Kind != yaml.MappingNode {
			return 0, false, fmt.Errorf("modules: item %d is no mapping", index+1)
		}
		_, value, err := mappingKey(item, "directory")
		if err != nil {
			return 0, false, fmt.Errorf("modules: item %d: %w", index+1, err)
		}
		if value != nil && value.Value == directory {
			return index, true, nil
		}
	}
	return 0, false, nil
}

// moduleItem is where an item of the modules list lies: its lines, from
// first to last, the column of its dash, and the text from the dash to its
// first key, "- " most often.
type moduleItem struct {
	node       *yaml.Node
	first      int
	last       int
	dashColumn int
	dashPrefix string
}

// itemOf locates the item whose mapping is node, from its dash, on the line
// of its first key or alone on a line above it.
func (c *configText) itemOf(node *yaml.Node) (moduleItem, error) {
	line, column := node.Line-1, c.offset(node)
	before := c.lines[line].text[:column]
	if strings.TrimSpace(before) == "-" {
		dash := strings.Index(before, "-")
		return moduleItem{node: node, first: line, last: c.blockEnd(line, dash), dashColumn: dash, dashPrefix: before[dash:]}, nil
	}
	for index := line - 1; index >= 0 && strings.TrimSpace(before) == ""; index-- {
		text := c.lines[index].text
		if !isContent(text) {
			continue
		}
		if strings.TrimSpace(text) != "-" {
			break
		}
		dash := indentation(text)
		return moduleItem{node: node, first: index, last: c.blockEnd(index, dash), dashColumn: dash, dashPrefix: "- "}, nil
	}
	return moduleItem{}, fmt.Errorf("modules: the item at line %d has no dash bumpit can find", node.Line)
}

// appendModules adds a modules key that declares declaration at the end of
// the file.
func (c *configText) appendModules(declaration ModuleDeclaration) ([]byte, error) {
	column := c.rootColumn()
	lines, err := moduleLines(declaration, column+2, "- ")
	if err != nil {
		return nil, err
	}
	c.insert(len(c.lines), append([]string{strings.Repeat(" ", column) + "modules:"}, lines...))
	return c.bytes(), nil
}

// firstModule puts declaration under a modules key that lists none, which
// loses the [] or the null it was written with.
func (c *configText) firstModule(section modulesSection, declaration ModuleDeclaration) ([]byte, error) {
	keyLine := section.key.Line - 1
	if start := c.offset(section.value); isWritten(section.value) {
		line := &c.lines[keyLine]
		line.text = strings.TrimRight(line.text[:start], " \t") + line.text[writtenEnd(line.text, start, section.value):]
	}

	lines, err := moduleLines(declaration, section.key.Column-1+2, "- ")
	if err != nil {
		return nil, err
	}
	c.insert(keyLine+1, lines)
	return c.bytes(), nil
}

// isWritten reports an empty modules value written out, [] or a null such
// as ~, rather than left out after the key.
func isWritten(value *yaml.Node) bool {
	return value.Kind == yaml.SequenceNode || value.Value != ""
}

// writtenEnd is the index after the empty modules value written out from
// start on its line.
func writtenEnd(text string, start int, value *yaml.Node) int {
	if value.Kind == yaml.SequenceNode {
		return start + strings.Index(text[start:], "]") + 1
	}
	return start + len(value.Value)
}

// moduleLines is the declaration as an item of the modules list, its dash
// at dashColumn followed by dashPrefix and the keys in the order the README
// gives them, the lists written in [...].
func moduleLines(declaration ModuleDeclaration, dashColumn int, dashPrefix string) ([]string, error) {
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	mapping.Content = append(mapping.Content, yamlString("directory"), yamlString(declaration.Directory))
	if declaration.TagPrefix != "" {
		mapping.Content = append(mapping.Content, yamlString("tagPrefix"), yamlString(declaration.TagPrefix))
	}
	if len(declaration.Paths) > 0 {
		mapping.Content = append(mapping.Content, yamlString("paths"), flowList(declaration.Paths))
	}
	if declaration.ShippedBy != nil {
		mapping.Content = append(mapping.Content, yamlString("shippedBy"), flowList(*declaration.ShippedBy))
	}

	lines, err := encodeYAML(&yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{mapping}})
	if err != nil {
		return nil, err
	}
	margin := strings.Repeat(" ", dashColumn)
	for index, line := range lines {
		if index == 0 {
			lines[index] = margin + dashPrefix + strings.TrimPrefix(line, "- ")
			continue
		}
		lines[index] = margin + strings.Repeat(" ", len(dashPrefix)) + strings.TrimPrefix(line, "  ")
	}
	return lines, nil
}

func flowList(values []string) *yaml.Node {
	list := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, value := range values {
		list.Content = append(list.Content, yamlString(value))
	}
	return list
}
