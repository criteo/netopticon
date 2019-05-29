package snmpmagic

import (
	"fmt"
	"log"
	"reflect"
)

import (
	"github.com/soniah/gosnmp"
)

func deserializePDUToValue(pdu *gosnmp.SnmpPDU, value reflect.Value, fieldName string) {
	var expectedFieldType string

	// TODO: add a type conversion flag (possibly with per-type options)
	switch pdu.Type {
	case gosnmp.Integer:
		intVal, ok := pdu.Value.(int64)
		if !ok {
			intVal = int64(pdu.Value.(int))
		}
		switch value.Kind() {
		case reflect.Bool:
			if intVal == 1 {
				value.SetBool(true)
			} else if intVal == 2 {
				value.SetBool(false)
			} else {
				panic(fmt.Sprint("Value cannot be converted to bool: ", intVal))
			}

		case reflect.Int, reflect.Int32, reflect.Int64:
			value.SetInt(intVal)

		default:
			expectedFieldType = "{int, int32, int64}"
		}

	case gosnmp.Counter32, gosnmp.Counter64:
		fallthrough
	case gosnmp.Gauge32:
		fallthrough
	case gosnmp.TimeTicks:
		fallthrough
	case gosnmp.Uinteger32:
		uintVal, ok := pdu.Value.(uint64)
		if !ok {
			uintVal = uint64(pdu.Value.(uint))
		}
		switch value.Kind() {
		case reflect.Bool:
			if uintVal == 1 {
				value.SetBool(true)
			} else if uintVal == 2 {
				value.SetBool(false)
			} else {
				panic(fmt.Sprint("Value cannot be converted to bool: ", uintVal))
			}

		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			value.SetUint(uintVal)

		default:
			expectedFieldType = "{uint, uint32, uint64}"
		}

	case gosnmp.OctetString:
		bytesVal := pdu.Value.([]byte)
		switch value.Kind() {
		case reflect.Slice:
			if value.Type().Elem().Kind() == reflect.Uint8 {
				bytesValCopy := make([]byte, len(bytesVal))
				copy(bytesValCopy, bytesVal)
				value.SetBytes(bytesValCopy)
			} else {
				panic(fmt.Sprint("Expected []byte but got ", value.Type()))
			}

		case reflect.String:
			value.SetString(string(bytesVal))

		default:
			expectedFieldType = "string"
		}

	case gosnmp.ObjectIdentifier:
		if value.Kind() == reflect.String {
			value.SetString(pdu.Value.(string))
		} else if value.Kind() == reflect.Slice && value.Type() == reflect.TypeOf(OID{}) {
			// FIXME: error handling + handle relative OIDs!
			if oid, err := ParseOID(pdu.Value.(string)); err == nil {
				value.Set(reflect.ValueOf(oid))
			}
		} else {
			expectedFieldType = "{snmpmagic.OID,string}"
		}

	default:
		log.Println("UNHANDLED:", fieldName, pdu.Name, pdu.Type, reflect.TypeOf(pdu.Value))
	}

	if expectedFieldType != "" {
		log.Printf(
			"%s is %v but should be %s",
			fieldName,
			value.Kind(),
			expectedFieldType,
		)
	}
}

func getOrCreateMapElement(value reflect.Value, fieldQualifiedName string, path OID) (
	elem reflect.Value, remainder OID, err error,
) {
	valueType := value.Type()
	if valueType.Kind() != reflect.Map {
		err = fmt.Errorf(
			"snmpmagic: suffix-catching fields must be a map (got %v)",
			valueType,
		)
		return
	}

	mapElemType := valueType.Elem()
	if mapElemType.Kind() != reflect.Ptr {
		err = fmt.Errorf(
			"snmpmagic: suffix-catching map element must be a struct pointer (got %v)",
			mapElemType,
		)
		return
	}

	if value.IsNil() {
		if !value.CanSet() {
			err = fmt.Errorf(
				"snmpmagic: cannot set value of field '%s'",
				fieldQualifiedName,
			)
			return
		}

		newMap := reflect.MakeMap(valueType)
		value.Set(newMap)
	}

	// TODO: check for custom index possibility
	//       (if -2, then should at least have 2 elements, etc.)
	if len(path) == 0 {
		err = fmt.Errorf(
			"snmpmagic: reached suffix-catching node with no path elements left",
		)
		return
	}

	mapKeyIndex := len(path) - 1 // TODO: make customizable
	mapKey := path[mapKeyIndex]
	remainder = path[:mapKeyIndex]

	var mapKeyValue reflect.Value
	switch valueType.Key().Kind() {
	case reflect.String:
		mapKeyValue = reflect.ValueOf(fmt.Sprint(mapKey))

	case reflect.Uint:
		mapKeyValue = reflect.ValueOf(mapKey)

	default:
		err = fmt.Errorf(
			"snmpmagic: suffix-catching map key must be {string,uint} (got %v)",
			valueType.Key(),
		)
		return
	}

	// Check existence of map element, create and insert if not present.
	mapElem := value.MapIndex(mapKeyValue)
	if !mapElem.IsValid() || mapElem.IsNil() {
		// We ensured this is a pointer above.
		mapElemType = mapElemType.Elem()
		mapElem = reflect.New(mapElemType)
		value.SetMapIndex(mapKeyValue, mapElem)
	}

	// Dereference pointer to map element.
	elem = mapElem.Elem()

	return
}
