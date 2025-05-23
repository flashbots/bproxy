package utils

import "strings"

func MapHasKeyWithPrefix[V any](m map[string]V, prefix string) bool {
	for key := range m {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
