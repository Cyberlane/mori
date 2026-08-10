package corpus

import "strings"

func LooksLikeEmail(value string) bool {
	cleaned := strings.TrimSpace(value)
	return strings.Contains(cleaned, "@") && strings.Contains(cleaned, ".")
}

func SumValues(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}
