package talos

import "errors"

// ErrReadOnly is returned by every mutating Client method while the client is
// read-only (the default). It is the second safety net behind the UI guard: a
// bug in the UI must not be able to change a cluster.
var ErrReadOnly = errors.New("read-only mode: start t9s with --write to change the cluster")

// MutatingMethods lists every Client method that changes a cluster. Each one
// calls c.refuseWrite first and starts no subprocess when it fails.
var MutatingMethods = []string{
	"Reboot", "Shutdown", "UpgradeTalos", "UpgradeK8s", "ApplyConfig", "PatchMachineConfig",
}

// SetWritable lets the mutating methods run (--write). The zero value of a
// Client is read-only.
func (c *Client) SetWritable(w bool) {
	c.mu.Lock()
	c.writable = w
	c.mu.Unlock()
}

// ReadOnly reports whether mutating methods are refused.
func (c *Client) ReadOnly() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.writable
}

func (c *Client) refuseWrite() error {
	if c.ReadOnly() {
		return ErrReadOnly
	}
	return nil
}
