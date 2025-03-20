package ignore

import (
	"strings"

	"github.com/bloodmagesoftware/zet/internal/project"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

func GetMatcher(p project.Project) gitignore.Matcher {
	ignoreLines := strings.Split(p.Ignore, "\n")
	patterns := make([]gitignore.Pattern, 0, len(ignoreLines)+2)
	for _, ignoreLine := range ignoreLines {
		ignoreLine := strings.TrimSpace(ignoreLine)
		if len(ignoreLine) == 0 || strings.HasPrefix(ignoreLine, "#") {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(ignoreLine, nil))
	}
	patterns = append(patterns, gitignore.ParsePattern(project.ProjectFileName, nil))
	patterns = append(patterns, gitignore.ParsePattern(".zet", nil))
	return gitignore.NewMatcher(patterns)
}
