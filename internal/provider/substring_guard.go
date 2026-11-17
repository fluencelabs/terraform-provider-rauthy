package provider

import "strings"

// Rauthy renames and deletes scopes, roles and groups by substring: it selects
// the clients or users whose comma-joined list contains the name anywhere and
// edits that string, so "ro" also hits "profile" and "dev" also hits
// "developer". An operation on a name is safe only when no other name of the
// same kind contains it.

// containingName returns another name in names that contains name, if any.
func containingName(name string, names []string) (string, bool) {
	for _, other := range names {
		if other != name && strings.Contains(other, name) {
			return other, true
		}
	}
	return "", false
}
