package main

import (
	"encoding/binary"
	"fmt"
)

// ┌────┬───┬────┬───┬────┬───┬───┬────┐
// │nstr│len│str1│len│str2│...│len│strn│
// └────┴───┴────┴───┴────┴───┴───┴────┘
const (
	K_MAX_ARGS = 32
)

// deserializes the request bytes into a slice of strings
func ParseReq(bytes []byte) ([]string, error) {
	offset := 0
	nstr := binary.LittleEndian.Uint32(bytes[offset : offset+4])
	if nstr > K_MAX_ARGS {
		return nil, fmt.Errorf("error: maximum number of arguments exceeded: %d", nstr)
	}
	offset += 4

	results := make([]string, nstr)
	r_len := nstr
	for nstr > 0 {
		if offset+4 > len(bytes) {
			return nil, fmt.Errorf("error: invalid argument length: %d", offset+4)
		}
		// n is the length of string tobe read
		n := int(binary.LittleEndian.Uint32(bytes[offset : offset+4]))
		offset += 4

		if offset+int(n) > len(bytes) {
			return nil, fmt.Errorf("error: invalid argument length: %d", offset+(n))
		}
		// s is the string tobe read
		s := make([]byte, n)
		copy(s, bytes[offset:offset+n])
		offset += n

		results[r_len - nstr] = string(s)
		nstr -= 1
	}

	if offset != len(bytes) {
		return nil, fmt.Errorf("error: invalid argument: %d", offset)
	}
	return results, nil
}
