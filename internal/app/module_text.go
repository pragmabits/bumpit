package app

import (
	"fmt"
	"slices"
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

// setModuleText writes the declaration in place of the item at index of the
// modules list. It refuses an item that holds a comment, which the new text
// would lose.
func setModuleText(content []byte, index int, declaration ModuleDeclaration) ([]byte, error) {
	text, _, item, err := parseModuleItem(content, index)
	if err != nil {
		return nil, err
	}
	if text.holdsComment(item.first, item.last, item.node) {
		return nil, fmt.Errorf("the declaration of %s holds a comment, which bumpit modules set would lose: edit it by hand", declaration.identity())
	}
	lines, err := moduleLines(declaration, item.dashColumn, item.dashPrefix)
	if err != nil {
		return nil, err
	}
	text.replace(item.first, item.last, lines)
	return text.bytes(), nil
}

// setCarriersText gives the item at index of the modules list the carriers, in
// place of the value of its shippedBy key, and leaves the rest of the item as
// it is, comments included. A value its line does not hold whole is refused.
func setCarriersText(content []byte, index int, carriers []string) ([]byte, error) {
	text, _, item, err := parseModuleItem(content, index)
	if err != nil {
		return nil, err
	}
	key, value, err := mappingKey(item.node, "shippedBy")
	if err != nil {
		return nil, err
	}
	if key == nil || !isOnOneLine(value) || value.Line != key.Line || text.valueEnd(value) < 0 {
		return nil, fmt.Errorf("shippedBy is written over several lines, which bumpit cannot edit: edit it by hand")
	}
	encoded, err := encodeYAML(flowList(carriers))
	if err != nil {
		return nil, err
	}

	line := &text.lines[value.Line-1]
	start := text.offset(value)
	line.text = line.text[:start] + encoded[0] + line.text[text.valueEnd(value):]
	return text.bytes(), nil
}

// isOnOneLine reports a scalar, or a flow list whose items lie on its line.
func isOnOneLine(value *yaml.Node) bool {
	switch {
	case value.Kind == yaml.ScalarNode:
		return true
	case value.Kind != yaml.SequenceNode || value.Style&yaml.FlowStyle == 0:
		return false
	default:
		return !slices.ContainsFunc(value.Content, func(item *yaml.Node) bool { return item.Line != value.Line })
	}
}

// valueEnd is the index after value, which isOnOneLine accepted, on its line, and
// -1 for a flow list that closes on a later line.
func (c *configText) valueEnd(value *yaml.Node) int {
	text := c.lines[value.Line-1].text
	if value.Kind == yaml.ScalarNode {
		return scalarEnd(text, c.offset(value))
	}
	from := c.offset(value)
	if len(value.Content) > 0 {
		last := value.Content[len(value.Content)-1]
		from = c.offset(last) + len(last.Value)
		if last.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle) != 0 {
			from = closingQuote(text, c.offset(last))
		}
	}
	closing := strings.Index(text[from:], "]")
	if closing < 0 {
		return -1
	}
	return from + closing + 1
}

// removeModuleText removes the item at index of the modules list, the
// comments inside it included. Removing the last one leaves modules: [], which
// still declares that the repository has no other module.
func removeModuleText(content []byte, index int) ([]byte, error) {
	text, section, item, err := parseModuleItem(content, index)
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

// parseModuleItem locates the item at index of the modules list, the place
// the same declaration has in the config the text loads as.
func parseModuleItem(content []byte, index int) (*configText, modulesSection, moduleItem, error) {
	text, section, err := parseModules(content)
	if err != nil {
		return nil, modulesSection{}, moduleItem{}, err
	}
	if section.key == nil || index >= len(section.value.Content) {
		return nil, modulesSection{}, moduleItem{}, fmt.Errorf("modules: the file has no item %d", index+1)
	}
	node := section.value.Content[index]
	if node.Kind != yaml.MappingNode {
		return nil, modulesSection{}, moduleItem{}, fmt.Errorf("modules: item %d is no mapping", index+1)
	}
	item, err := text.itemOf(node)
	return text, section, item, err
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
	if declaration.Name != "" {
		mapping.Content = append(mapping.Content, yamlString("name"), yamlString(declaration.Name))
	}
	mapping.Content = append(mapping.Content, yamlString("directories"), flowList(declaration.Directories))
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
