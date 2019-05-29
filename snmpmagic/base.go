package snmpmagic

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync/atomic"
)

import (
	"github.com/soniah/gosnmp"
)

type SNMPMagic struct {
	// TODO: do we want concurrent run of bulkwalks when possible?

	oidTree     *OIDTree
	destination interface{}
	isFilled    int32
}

func NewSNMPMagic(dst interface{}) (*SNMPMagic, error) {
	oidTree, err := BuildOIDTree(dst)
	if err != nil {
		return nil, err
	}

	magic := &SNMPMagic{
		oidTree:     oidTree,
		destination: dst,
	}
	return magic, nil
}

func (self *SNMPMagic) String() string {
	var sb strings.Builder

	fmt.Fprintln(&sb, "BulkWalk queries:")
	for _, path := range self.oidTree.PrefixPaths() {
		fmt.Fprintln(&sb, "-", path)
	}
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, "OID Tree:")
	self.oidTree.prettyPrint(&sb, "")

	return sb.String()
}

func (self *SNMPMagic) Query(client *gosnmp.GoSNMP) error {
	if !atomic.CompareAndSwapInt32(&self.isFilled, 0, 1) {
		return errors.New("snmpmagic: structure has already been filled")
	}

	if err := client.Connect(); err != nil {
		return err
	}
	defer client.Conn.Close()

	rootOids := self.oidTree.PrefixPaths()
	for _, rootOid := range rootOids {
		err := client.BulkWalk(rootOid.String(), self.HandlePDU)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *SNMPMagic) HandlePDU(pdu gosnmp.SnmpPDU) error {
	path, err := ParseOID(pdu.Name)
	if err != nil {
		return err
	}

	remainder := path
	value := reflect.ValueOf(self.destination)
	node := self.oidTree
	for node != nil {
		// Dereference pointers.
		for value.Kind() == reflect.Ptr {
			value = value.Elem()
		}

		// Node has field index:
		// - move down the struct tree
		// - set value to corresponding field
		if node.fieldIndex >= 0 {
			value = value.Field(node.fieldIndex)
			if !value.IsValid() {
				return fmt.Errorf(
					"snmpmagic: field '%s' is invalid (not a pointer?)",
					node.fieldQualifiedName,
				)
			}
		}

		// Node is suffix-catching:
		// - ensure map is initialized
		// - extract key suffix
		// - ensure element at key is initialized
		// - set value to element
		if node.IsSuffixCatching() {
			var err error
			value, remainder, err = getOrCreateMapElement(value, node.fieldQualifiedName, remainder)
			if err != nil {
				// We log an error and stop processing of the PDU instead of stopping
				// the whole walk.
				log.Println(
					"ERROR:", err, "at", node.fieldQualifiedName, "with OID", path,
				)
				return nil
			}
		}

		// Node is a leaf: check types and deserialize PDU.
		if node.IsLeaf() {
			if len(remainder) == 0 {
				deserializePDUToValue(&pdu, value, node.fieldQualifiedName)
				return nil
			} else {
				// TODO: log erroneous data (or schema)?
			}
		}

		node, remainder = node.FindNext(remainder)
	}

	return nil
}
