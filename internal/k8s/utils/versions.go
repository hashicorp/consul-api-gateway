package utils

import "strconv"

func ResourceVersionGreater(a, b string) bool {
	aVal, err := strconv.Atoi(a)
	if err != nil {
		// a isn't numeric, return that b is greater
		return false
	}
	bVal, err := strconv.Atoi(b)
	if err != nil {
		// b isn't numeric, return that a is greater
		return true
	}
	return aVal > bVal
}
