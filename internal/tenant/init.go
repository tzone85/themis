package tenant

import (
	"fmt"
	"os"
)

// Init creates the tenant directory tree on disk. It is idempotent:
// re-running on an existing tenant returns the same Tenant without error.
//
// Directory permissions: 0o700 (rwx for owner only). The Themis daemon
// must run as a single user; cross-user access on the host is out of scope.
func Init(base, id string) (Tenant, error) {
	t, err := New(base, id)
	if err != nil {
		return Tenant{}, err
	}
	for _, dir := range []string{t.Root(), t.BOM(), t.Wing()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Tenant{}, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return t, nil
}
