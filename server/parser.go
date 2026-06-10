package main

import (
	"fmt"
	"strings"
)

func ParseReq(data []byte) ([]string, error) {
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty request")
	}
	return fields, nil
}
