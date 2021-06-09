package main

import (
	"fmt"
	"strings"
)

func ParseFormats(in string) (map[string]string, error) {
	result := make(map[string]string)
	for _, s := range strings.Split(in, "\n") {
		pos := strings.Index(s, "#")
		if pos >= 0 {
			s = s[:pos]
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		tokens := strings.Split(s, "=")
		if len(tokens) != 2 {
			return result, fmt.Errorf("Bad format for setting: %s", s)
		}
		result[tokens[0]] = tokens[1]
	}
	return result, nil	
}
