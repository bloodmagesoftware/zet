//go:build windows

package paths

import (
	"path/filepath"
	"strings"
)

func (p System) ToUnix() Unix {
	pStr := string(p)
	if filepath.IsAbs(pStr) {
		drive := strings.ToLower(string(pStr[0]))
		pathWithoutDrive := pStr[2:]
		pathWithoutDrive = strings.ReplaceAll(pathWithoutDrive, "\\", "/")
		return Unix("/" + drive + pathWithoutDrive)
	} else {
		return Unix(strings.ReplaceAll(pStr, "\\", "/"))
	}
}
