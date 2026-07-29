// Package diagnostic formats errors without exposing private filesystem paths.
package diagnostic

import (
	"errors"
	"fmt"
	"os"
)

// Message removes path operands from structured operating-system errors. The
// caller reports the already-sanitized display path separately.
func Message(err error) string {
	if err == nil {
		return ""
	}

	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return fmt.Sprintf("%s: %v", pathError.Op, pathError.Err)
	}

	var linkError *os.LinkError
	if errors.As(err, &linkError) {
		return fmt.Sprintf("%s: %v", linkError.Op, linkError.Err)
	}

	return err.Error()
}
