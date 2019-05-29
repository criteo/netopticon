package snmpmagic

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode"
)

var oidTreeCacheByType sync.Map

// Builds an OID prefix tree from the given type and the tags on its fields.
// If a struct is passed, its type will be extracted first. Results are cached
// in concurrency-safe way.
func BuildOIDTree(x interface{}) (*OIDTree, error) {
	t, ok := x.(reflect.Type)
	if !ok {
		t = reflect.TypeOf(x)
	}

	if cachedOidTree, ok := oidTreeCacheByType.Load(t); ok {
		return cachedOidTree.(*OIDTree), nil
	}

	oidTree := NewOIDTree()
	if err := oidTree.prepare(t, nil, ""); err != nil {
		return nil, err
	}

	cachedOidTree, _ := oidTreeCacheByType.LoadOrStore(t, oidTree)
	return cachedOidTree.(*OIDTree), nil
}

type OIDNodeType uint

const (
	UninitializedNode OIDNodeType = iota
	SimpleNode
	SuffixCatcherNode
	LeafNode
)

type OIDTree struct {
	isInitialized      bool
	prefix             OID
	children           map[uint]*OIDTree
	fieldIndex         int
	fieldQualifiedName string
	nodeType           OIDNodeType
}

func NewOIDTree() *OIDTree {
	return &OIDTree{
		prefix:             OID{},
		children:           make(map[uint]*OIDTree),
		fieldIndex:         -1,
		fieldQualifiedName: "",
		nodeType:           UninitializedNode,
	}
}

func (self *OIDTree) IsLeaf() bool {
	return self.nodeType == LeafNode
}

func (self *OIDTree) IsSuffixCatching() bool {
	return self.nodeType == SuffixCatcherNode
}

func (self *OIDTree) prepare(t reflect.Type, prefix OID, parentName string) error {
	// Dereference pointers.
	if t.Kind() == reflect.Ptr {
		self.prepare(t.Elem(), prefix, t.Elem().Name())
		return nil
	}

	for fieldIndex := 0; fieldIndex < t.NumField(); fieldIndex++ {
		field := t.Field(fieldIndex)

		snmpTag := field.Tag.Get("snmp")
		if snmpTag == "" {
			return nil
		}

		// Unexported fields cannot be set.
		// TODO: log warning - has SNMP tag but cannot be set
		if strings.IndexFunc(field.Name, unicode.IsLower) == 0 {
			continue
		}

		snmpTagOid, err := ParseOID(snmpTag)
		if err != nil {
			return err
		}

		path := append(prefix.Copy(), snmpTagOid...)
		if snmpTag[0] == '.' && len(prefix) > 0 {
			// TODO: log warning - ignoring absolute path outside of top level
		}

		fieldQualifiedName := parentName + "." + field.Name
		switch field.Type.Kind() {
		case reflect.Struct:
			self.Insert(path, fieldIndex, fieldQualifiedName, SimpleNode)
			self.prepare(field.Type, path, field.Type.Name())

		case reflect.Map:
			self.Insert(path, fieldIndex, fieldQualifiedName, SuffixCatcherNode)
			self.prepare(field.Type.Elem(), path, field.Type.Elem().Name())

		default:
			self.Insert(path, fieldIndex, fieldQualifiedName, LeafNode)
		}
	}

	return nil
}

func (self *OIDTree) createOrUpdateChild(path OID, fieldIndex int, fieldQualifiedName string, nodeType OIDNodeType) {
	key := path[0]
	childPath := path[1:]
	if child, ok := self.children[key]; ok {
		child.Insert(childPath, fieldIndex, fieldQualifiedName, nodeType)
	} else if self.IsLeaf() {
		panic("snmpmagic: oidtree: cannot insert node under a leaf")
	} else {
		self.children[key] = &OIDTree{
			prefix:             childPath.Copy(),
			children:           make(map[uint]*OIDTree),
			fieldIndex:         fieldIndex,
			fieldQualifiedName: fieldQualifiedName,
			nodeType:           nodeType,
		}
	}
}

func (self *OIDTree) String() string {
	var sb strings.Builder
	self.prettyPrint(&sb, "")
	return sb.String()
}

func (self *OIDTree) prettyPrint(sb *strings.Builder, indent string) {
	fmt.Fprint(sb, indent, self.prefix, " ", self.fieldQualifiedName, " ")
	if self.IsSuffixCatching() {
		sb.WriteString("<suffix-catching> ")
	}
	if self.IsLeaf() {
		sb.WriteString("<leaf>")
	}
	sb.WriteRune('\n')

	for key, child := range self.children {
		fmt.Fprintf(sb, "%s[%v]\n", indent, key)
		child.prettyPrint(sb, indent+"  ")
	}
}

func (self *OIDTree) Insert(path OID, fieldIndex int, fieldQualifiedName string, nodeType OIDNodeType) {
	if self.nodeType == UninitializedNode {
		self.prefix = path.Copy()
		self.fieldIndex = fieldIndex
		self.fieldQualifiedName = fieldQualifiedName
		self.nodeType = nodeType
		return
	}

	commonLen := self.prefix.LongestCommonPrefixLength(path)

	// Check whether can just insert a child node.
	if commonLen == len(self.prefix) && commonLen < len(path) {
		self.createOrUpdateChild(
			path[commonLen:], fieldIndex, fieldQualifiedName, nodeType,
		)
		return
	}

	// Ensure we have at least 1 element for child node key.
	if commonLen == len(path) {
		commonLen -= 1
	}

	// Move ourselves down the tree.
	self.children = map[uint]*OIDTree{
		self.prefix[commonLen]: &OIDTree{
			prefix:             self.prefix[commonLen+1:],
			children:           self.children,
			fieldIndex:         self.fieldIndex,
			fieldQualifiedName: self.fieldQualifiedName,
			nodeType:           self.nodeType,
		},
	}

	// After split, we get converted into a logical node:
	// neither a leaf nor suffix-catching.
	self.prefix = self.prefix[:commonLen]
	self.fieldIndex = -1
	self.fieldQualifiedName = ""
	self.nodeType = SimpleNode

	// Insert new child.
	self.createOrUpdateChild(
		path[commonLen:], fieldIndex, fieldQualifiedName, nodeType,
	)
}

func (self *OIDTree) FindNext(path OID) (next *OIDTree, remainder OID) {
	prefixLen := self.prefix.LongestCommonPrefixLength(path)
	if prefixLen != len(self.prefix) {
		return nil, path
	}

	remainingPath := path[prefixLen:]
	if len(remainingPath) == 0 {
		return self, nil
	} else {
		key := remainingPath[0]
		if child, ok := self.children[key]; ok {
			return child, remainingPath[1:]
		}
	}

	return nil, path
}

func (self *OIDTree) PrefixPaths() (paths []OID) {
	if self.IsLeaf() {
		return
	}

	if self.IsSuffixCatching() {
		paths = append(paths, self.prefix.Copy())
		return
	}

	for key, child := range self.children {
		for _, path := range child.PrefixPaths() {
			prefixPath := self.prefix.Copy()
			prefixPath = append(prefixPath, key)
			prefixPath = append(prefixPath, path...)

			paths = append(paths, prefixPath)
		}
	}

	return
}
