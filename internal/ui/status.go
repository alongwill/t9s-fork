package ui

// helpStatusSection is the "Status" block at the top of the help overlay: the
// mode, and (later milestones) the Omni detection and the current role.
func (app App) helpStatusSection(section func(string, [][2]string) string) string {
	mode := "read-only (default): keys that change a cluster are hidden and refused"
	if app.writeMode {
		mode = "write (--write): R S U K and machine config edit are enabled"
	}
	rows := [][2]string{{"mode", mode}}
	return section("Status", rows)
}
