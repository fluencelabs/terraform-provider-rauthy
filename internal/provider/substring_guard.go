package provider

import (
	"strings"
	"sync"
)

// substringMutationMu keeps substring checks and their destructive writes atomic
// relative to provider writes that can introduce names into the affected lists.
// It spans all configured clients in this process. External Rauthy writers are
// outside this lock.
var substringMutationMu sync.Mutex //nolint:gochecknoglobals // All resource instances must share this mutation lock.

// Rauthy edits the comma-joined scope lists of clients and role and group
// assignments of users, and user attribute names in scope mappings by substring. A rename replaces every occurrence of the
// old name, so renaming "ro" also rewrites "profile". A deletion cuts ",name" out of
// the string, so deleting "dev" also cuts the start of a later "developer";
// when the string starts with the name, it cuts "name," instead, so deleting
// "dev" also cuts the end of "xdev" followed by another entry. These helpers find the other name such an edit would damage.

// containingName returns another name that contains name: the names a rename
// of name would rewrite.
func containingName(name string, names []string) (string, bool) {
	for _, other := range names {
		if other != name && strings.Contains(other, name) {
			return other, true
		}
	}
	return "", false
}

// cutName returns another name that starts or ends with name: the names a
// deletion of name would cut.
func cutName(name string, names []string) (string, bool) {
	for _, other := range names {
		if other != name && (strings.HasPrefix(other, name) || strings.HasSuffix(other, name)) {
			return other, true
		}
	}
	return "", false
}
