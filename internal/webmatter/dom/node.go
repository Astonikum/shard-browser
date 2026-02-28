// Package dom implements the Document Object Model (DOM) for the WebMatter engine.
package dom

import "strings"

// NodeType identifies the type of a DOM node.
type NodeType uint8

const (
	NodeTypeDocument NodeType = 9
	NodeTypeDoctype  NodeType = 10
	NodeTypeElement  NodeType = 1
	NodeTypeText     NodeType = 3
	NodeTypeComment  NodeType = 8
)

// Node is a node in the DOM tree.
type Node struct {
	Type     NodeType
	TagName  string            // lowercase, for elements
	Attrs    map[string]string // element attributes
	Data     string            // text/comment content, or doctype name

	Parent   *Node
	Children []*Node

	// Styling
	ComputedStyle *ComputedStyle

	// Layout (set by layout engine)
	LayoutBox interface{}
}

// Document is the root of an HTML document.
type Document struct {
	Node
	Root     *Node  // <html>
	Head     *Node  // <head>
	Body     *Node  // <body>
	BaseURL  string
	Title    string
}

// NewDocument creates an empty document.
func NewDocument() *Document {
	d := &Document{}
	d.Node.Type = NodeTypeDocument
	d.Node.TagName = "#document"
	return d
}

// NewElement creates an element node.
func NewElement(tagName string) *Node {
	return &Node{
		Type:    NodeTypeElement,
		TagName: tagName,
		Attrs:   make(map[string]string),
	}
}

// NewText creates a text node.
func NewText(data string) *Node {
	return &Node{Type: NodeTypeText, Data: data}
}

// NewComment creates a comment node.
func NewComment(data string) *Node {
	return &Node{Type: NodeTypeComment, Data: data}
}

// AppendChild adds a child node.
func (n *Node) AppendChild(child *Node) {
	child.Parent = n
	n.Children = append(n.Children, child)
}

// InsertBefore inserts newChild before refChild. If refChild is nil, appends.
func (n *Node) InsertBefore(newChild, refChild *Node) {
	newChild.Parent = n
	if refChild == nil {
		n.Children = append(n.Children, newChild)
		return
	}
	for i, c := range n.Children {
		if c == refChild {
			n.Children = append(n.Children[:i], append([]*Node{newChild}, n.Children[i:]...)...)
			return
		}
	}
	n.Children = append(n.Children, newChild)
}

// RemoveChild removes a child node.
func (n *Node) RemoveChild(child *Node) {
	for i, c := range n.Children {
		if c == child {
			n.Children = append(n.Children[:i], n.Children[i+1:]...)
			child.Parent = nil
			return
		}
	}
}

// GetAttr returns an attribute value.
func (n *Node) GetAttr(name string) string {
	if n.Attrs == nil {
		return ""
	}
	return n.Attrs[strings.ToLower(name)]
}

// SetAttr sets an attribute value.
func (n *Node) SetAttr(name, value string) {
	if n.Attrs == nil {
		n.Attrs = make(map[string]string)
	}
	n.Attrs[strings.ToLower(name)] = value
}

// HasAttr checks if attribute exists.
func (n *Node) HasAttr(name string) bool {
	if n.Attrs == nil {
		return false
	}
	_, ok := n.Attrs[strings.ToLower(name)]
	return ok
}

// HasClass checks if element has a CSS class.
func (n *Node) HasClass(class string) bool {
	for _, c := range strings.Fields(n.GetAttr("class")) {
		if c == class {
			return true
		}
	}
	return false
}

// Classes returns the list of CSS classes.
func (n *Node) Classes() []string {
	return strings.Fields(n.GetAttr("class"))
}

// ID returns the element's id attribute.
func (n *Node) ID() string {
	return n.GetAttr("id")
}

// TextContent returns the text content of the node and all descendants.
func (n *Node) TextContent() string {
	if n.Type == NodeTypeText {
		return n.Data
	}
	var b strings.Builder
	for _, child := range n.Children {
		b.WriteString(child.TextContent())
	}
	return b.String()
}

// FirstChild returns the first child or nil.
func (n *Node) FirstChild() *Node {
	if len(n.Children) == 0 {
		return nil
	}
	return n.Children[0]
}

// LastChild returns the last child or nil.
func (n *Node) LastChild() *Node {
	if len(n.Children) == 0 {
		return nil
	}
	return n.Children[len(n.Children)-1]
}

// NextSibling returns the next sibling or nil.
func (n *Node) NextSibling() *Node {
	if n.Parent == nil {
		return nil
	}
	for i, s := range n.Parent.Children {
		if s == n && i+1 < len(n.Parent.Children) {
			return n.Parent.Children[i+1]
		}
	}
	return nil
}

// PrevSibling returns the previous sibling or nil.
func (n *Node) PrevSibling() *Node {
	if n.Parent == nil {
		return nil
	}
	for i, s := range n.Parent.Children {
		if s == n && i > 0 {
			return n.Parent.Children[i-1]
		}
	}
	return nil
}

// QuerySelector finds the first element matching a simple CSS selector.
func (n *Node) QuerySelector(sel string) *Node {
	var result *Node
	Walk(n, func(node *Node) bool {
		if result != nil {
			return false
		}
		if node.Type == NodeTypeElement && matchesSimpleSelector(node, sel) {
			result = node
			return false
		}
		return true
	})
	return result
}

// QuerySelectorAll finds all elements matching a simple CSS selector.
func (n *Node) QuerySelectorAll(sel string) []*Node {
	var results []*Node
	Walk(n, func(node *Node) bool {
		if node.Type == NodeTypeElement && matchesSimpleSelector(node, sel) {
			results = append(results, node)
		}
		return true
	})
	return results
}

func matchesSimpleSelector(n *Node, sel string) bool {
	if sel == "" {
		return false
	}
	switch sel[0] {
	case '#':
		return n.ID() == sel[1:]
	case '.':
		return n.HasClass(sel[1:])
	default:
		return n.TagName == strings.ToLower(sel)
	}
}

// Walk performs a depth-first traversal. Return false from fn to skip children.
func Walk(n *Node, fn func(*Node) bool) {
	if !fn(n) {
		return
	}
	for _, child := range n.Children {
		Walk(child, fn)
	}
}

// GetElementByID finds an element by id.
func GetElementByID(root *Node, id string) *Node {
	return root.QuerySelector("#" + id)
}

// GetElementsByTagName finds all elements with the given tag name.
func GetElementsByTagName(root *Node, tag string) []*Node {
	tag = strings.ToLower(tag)
	var results []*Node
	Walk(root, func(n *Node) bool {
		if n.Type == NodeTypeElement && (tag == "*" || n.TagName == tag) {
			results = append(results, n)
		}
		return true
	})
	return results
}
