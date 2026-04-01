package trasher

import (
	"fmt"
	"strings"
)

// Criterion determines whether a message should be included for trashing.
type Criterion interface {
	// Match returns true if the given email address matches this criterion.
	Match(emailAddress string) bool
	// Description returns a human-readable summary for confirmation prompts.
	Description() string
}

// FromAddress matches messages sent from a specific email address.
type FromAddress struct {
	Address string
}

func (f FromAddress) Match(emailAddress string) bool {
	return strings.EqualFold(emailAddress, f.Address)
}

func (f FromAddress) Description() string {
	return fmt.Sprintf("messages from %s", f.Address)
}
