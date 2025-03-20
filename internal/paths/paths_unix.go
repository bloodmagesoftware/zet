//go:build !windows

package paths

func (p System) ToUnix() Unix {
	return Unix(p)
}
