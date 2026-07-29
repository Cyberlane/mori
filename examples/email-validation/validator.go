package emailvalidation

import "strings"

func LooksLikeEmail(address string) bool {
	cleaned := strings.TrimSpace(address)
	return strings.Contains(cleaned, "@") && strings.Contains(cleaned, ".")
}
