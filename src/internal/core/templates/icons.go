package templates

// iconStyleSuffix returns the Bootstrap Icons class-name suffix for a given
// icon style. Outline maps to BI's default (no suffix); Fill maps to "-fill",
// matching BI's `bi-{name}-fill` filled-variant convention.
func iconStyleSuffix(style IconStyle) string {
	if style == IconStyleFill {
		return "-fill"
	}
	return ""
}
