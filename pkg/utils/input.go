package utils

import (
	"strings"
)

func ParsePeers(peersString string) []string {
	if peersString == "" {
		return nil
	}
	return strings.Split(peersString, ",")
}
