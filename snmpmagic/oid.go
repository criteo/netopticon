package snmpmagic

import (
	"fmt"
	"strconv"
	"strings"
)

type OID []uint

func ParseOID(str string) (OID, error) {
	// Split into OID path elements, drop leading dot(s)
	parts := strings.Split(str, ".")
	for len(parts) > 0 && parts[0] == "" {
		parts = parts[1:]
	}

	oid := make(OID, len(parts))
	for i, part := range parts {
		if partUint, err := strconv.ParseUint(part, 10, 64); err != nil {
			return nil, err
		} else {
			oid[i] = uint(partUint)
		}
	}

	return oid, nil
}

func (self OID) String() string {
	var sb strings.Builder

	for i := 0; i < len(self); i++ {
		fmt.Fprint(&sb, self[i])

		if i != len(self)-1 {
			sb.WriteRune('.')
		}
	}

	return sb.String()
}

func (self OID) Copy() OID {
	clone := make(OID, len(self))
	copy(clone, self)
	return clone
}

func (self OID) LongestCommonPrefixLength(other OID) int {
	minLength := len(self)
	if len(other) < minLength {
		minLength = len(other)
	}

	prefixLen := 0
	for prefixLen < minLength {
		if self[prefixLen] != other[prefixLen] {
			break
		}

		prefixLen += 1
	}

	return prefixLen
}
