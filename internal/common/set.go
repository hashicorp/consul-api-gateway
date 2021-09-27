package common

// Difference computes the resources found in b, but not in a
func Difference(a, b []string) []string {
	results := []string{}
	for _, entry := range b {
		for _, found := range a {
			if found == entry {
				results = append(results, entry)
			}
		}
	}
	return results
}
