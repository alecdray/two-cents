package templates

import (
	"fmt"
	"os"
	"strings"
)

var cssVersion string

// InitCSSVersion sets the CSS cache-busting version from the mod time of the given file path.
func InitCSSVersion(path string) {
	if info, err := os.Stat(path); err == nil {
		cssVersion = fmt.Sprintf("%d", info.ModTime().Unix())
	}
}

func CreatePageTitle(title string) string {
	if title == "" {
		title = "Home"
	}

	return fmt.Sprintf("%s | Two Cents", title)
}

func FormatCallableID(id string) string {
	return strings.ReplaceAll(id, "-", "_")
}
