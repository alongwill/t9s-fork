package main

import "strings"

// resolveWrite decides whether write mode is on. Read-only is the default.
//
//	--write            on
//	--readonly=false   on
//	--readonly[=true]  off, even with T9S_READONLY=false (flags beat the environment)
//	T9S_READONLY=false on, only when neither flag was given
//
// Contradictory flags (--write with --readonly=true) stay read-only: the safe
// reading wins.
func resolveWrite(readOnly, write bool, set map[string]bool, env string) bool {
	if set["readonly"] && readOnly {
		return false
	}
	if set["write"] && write {
		return true
	}
	if set["readonly"] {
		return !readOnly
	}
	if set["write"] { // --write=false: an explicit flag beats the environment
		return false
	}
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "false", "0", "no", "off":
		return true
	}
	return false
}
