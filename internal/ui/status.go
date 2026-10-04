package ui

import "fmt"

// omniRoleNote explains how Omni decides the Talos role. It follows the
// router in the Omni source (internal/backend/grpc/router/talos_backend.go,
// setRoleHeaders; sensitive_read_guard.go).
var omniRoleNote = [][2]string{
	{"role", "via Omni: Omni maps your Omni user's role to a Talos role on every call"},
	{"", "Omni Reader: os:reader on every call."},
	{"", "Omni Operator or higher: os:operator on Talos 1.4+ (os:admin for a few methods:"},
	{"", "  disk wipe, LVM/MD removal, META writes, etcd downgrade/forfeit, debug container,"},
	{"", "  image remove; Copy/Read on Talos 1.12+)."},
	{"", "Reads of sensitive resources (MachineConfig, secrets) are always denied,"},
	{"", "  whatever your role: Omni's sensitive read guard refuses them."},
	{"", "  See omni internal/backend/grpc/router/talos_backend.go and sensitive_read_guard.go."},
}

// helpStatusSection is the "Status" block at the top of the help overlay: the
// mode, the platform (Omni or plain Talos) and where that came from, and the
// current role.
func (app App) helpStatusSection(section func(string, [][2]string) string) string {
	mode := "read-only (default): keys that change a cluster are hidden and refused"
	if app.writeMode {
		mode = "write (--write): R S U K and machine config edit are enabled"
	}
	rows := [][2]string{{"mode", mode}}
	if o := app.id.omni; o.Detected {
		text := "Omni"
		if o.Host != "" {
			text += " " + o.Host
		}
		rows = append(rows, [2]string{"platform", text + " (detected via " + o.Via + ")"})
	} else {
		rows = append(rows, [2]string{"platform", "Talos (no Omni detected)"})
	}
	switch {
	case app.id.viaOmni():
		rows = append(rows, omniRoleNote...)
	case app.id.hasCert:
		role := "role: " + app.id.roleSummary() + " (from the client certificate)"
		rows = append(rows, [2]string{"role", role})
		if days, expired, ok := app.id.cert.ExpiryWarning(app.id.clock(), certWarnWindow); ok {
			switch {
			case expired:
				rows = append(rows, [2]string{"", "the client certificate has expired"})
			default:
				rows = append(rows, [2]string{"", fmt.Sprintf("the client certificate expires in %d days", days)})
			}
		}
	}
	return section("Status", rows)
}
