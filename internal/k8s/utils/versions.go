package utils

import "strconv"

func ResourceVersionLesser(a, b string) bool {
	aVal, err := strconv.Atoi(a)
	if err != nil {
		// a isn't numeric, return that a is lesser
		return true
	}
	bVal, err := strconv.Atoi(b)
	if err != nil {
		// b isn't numeric, return that b is lesser
		return false
	}
	return aVal < bVal
}

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
