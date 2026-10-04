package ui

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
	return section("Status", rows)
}
