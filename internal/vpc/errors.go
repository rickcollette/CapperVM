package vpc

import (
	"errors"
	"strings"
)

// IsConstraintError reports SQLite UNIQUE / FK constraint failures, including
// CapDB driver errors that only expose rc=19 without a message.
func IsConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "FOREIGN KEY constraint failed") ||
		strings.Contains(msg, "constraint violation") ||
		strings.Contains(msg, "duplicate") {
		return true
	}
	// CapDB may return an empty errmsg with rc=19 (SQLITE_CONSTRAINT).
	return strings.Contains(msg, "(rc=19)")
}

// ErrVPCNameTaken is returned when a VPC name or slug is already in use.
var ErrVPCNameTaken = errors.New("vpc name already exists in project")
