package paths

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type (
	System string
	Git    []string
	Unix   string
	Path   interface {
		ToUnix() Unix
		ToString() string
		ToGit() Git
	}
)

func (p System) Hash() ([]byte, error) {
	f, err := p.Open()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to open file %s", p), err)
	}
	defer f.Close()

	h := sha256.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to read file %s", p), err)
	}

	return h.Sum(nil), nil
}

func (p System) ToGit() Git {
	return strings.Split(string(p), string(os.PathSeparator))
}

func (p System) Open() (*os.File, error) {
	return os.Open(string(p))
}

func (p System) Stat() (os.FileInfo, error) {
	return os.Stat(string(p))
}

func WalkDir(root System, fn func(sysPath System, d fs.DirEntry, err error) error) error {
	return filepath.WalkDir(string(root), func(path string, d fs.DirEntry, err error) error {
		return fn(System(path), d, err)
	})
}

func (p Unix) ToUnix() Unix {
	return p
}

func (p Unix) ToString() string {
	return string(p)
}

func (p Unix) ToGit() Git {
	return strings.Split(string(p), "/")
}

func (p Unix) CutSuffix(s, suffix Unix) (Unix, bool) {
	b, f := strings.CutSuffix(string(s), string(suffix))
	return Unix(b), f
}

func (p System) Rel(parent string) (System, error) {
	rel, err := filepath.Rel(parent, string(p))
	return System(rel), err
}

func (p Unix) Rel(parent string) (Unix, error) {
	rel, ok := strings.CutPrefix(string(p), parent)
	if !ok {
		return p, fmt.Errorf("path %s is not inside %s", p, parent)
	}
	rel, _ = strings.CutPrefix(rel, "/")
	return Unix(rel), nil
}

func (p System) ToString() string {
	return string(p)
}

func (p System) CutSuffix(s, suffix System) (System, bool) {
	b, f := strings.CutSuffix(string(s), string(suffix))
	return System(b), f
}

func (p Git) ToUnix() Unix {
	return Unix(strings.Join(p, "/"))
}

func (p Git) ToString() string {
	return strings.Join(p, "/")
}

func (p Git) CutSuffix(s, suffix Git) (Git, bool) {
	b, f := strings.CutSuffix(s.ToString(), suffix.ToString())
	return Unix(b).ToGit(), f
}

func (p Git) ToGit() Git {
	return p
}

func (p Git) Rel(parent string) (Git, error) {
	s, err := p.ToUnix().Rel(parent)
	return s.ToGit(), err
}
