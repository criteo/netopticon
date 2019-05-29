package main

import (
	"math"
	"strconv"
	"strings"
)

func wattsToDecibellMilliwatts(watts float32) float32 {
	// Simplified from 10 * log10(watts * 1000)
	return float32(10 * (3 + math.Log10(float64(watts))))
}

func interfaceNameToPort(name string) (uint, bool) {
	if strings.HasPrefix(name, "Ethernet") {
		// EthernetP or EthernetP/L
		name = name[8:]
		slashIdx := strings.IndexByte(name, '/')
		if slashIdx > 0 {
			name = name[:slashIdx]
		}

		if port, err := strconv.ParseUint(name, 10, 32); err == nil {
			return uint(port), true
		}
	} else if strings.HasPrefix(name, "et-") {
		// et-*/*/P
		// XXX: does not support multiple line cards, but should be OK.
		slashIdx := strings.LastIndexByte(name, '/')
		if slashIdx > 0 {
			name = name[slashIdx+1:]
		}

		// Should not be a virtual interface (e.g. et-0/0/0.0)
		if !strings.ContainsRune(name, '.') {
			if port, err := strconv.ParseUint(name, 10, 32); err == nil {
				// Juniper port numbering starts at 0
				return uint(port + 1), true
			}
		}
	}

	return ^uint(0), false
}
