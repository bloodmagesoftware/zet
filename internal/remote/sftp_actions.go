package remote

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/bloodmagesoftware/zet/internal/ignore"
	"github.com/bloodmagesoftware/zet/internal/options"
	"github.com/bloodmagesoftware/zet/internal/paths"
)

const (
	DirContent = "content"
	DirHash    = "hash"
	FileIgnore = "ignore_file"
)

type (
	commitFile struct {
		Path   paths.Path
		Status commitFileStatus
	}
	commitFileStatus uint8
)

const (
	commitFileStatusCreate commitFileStatus = iota
	commitFileStatusDelete
	commitFileStatusChange
)

func (cfs commitFileStatus) ToString() string {
	switch cfs {
	case commitFileStatusCreate:
		return "+"
	case commitFileStatusDelete:
		return "-"
	case commitFileStatusChange:
		return "~"
	default:
		return "?"
	}
}

func (r *Remote) IsEmpty() (bool, error) {
	fis, err := r.SftpClient.ReadDir(r.Config.Remote.Path)
	if err != nil {
		return false, errors.Join(fmt.Errorf("failed to read directory %s", r.Config.Remote.Path), err)
	}

	return len(fis) == 0, nil
}

func (r *Remote) InitialCommit() error {
	if options.FlagVerbose {
		fmt.Println("Initial commit")
	}

	if err := r.PushIgnore(); err != nil {
		return errors.Join(errors.New("failed to push ignore"), err)
	}

	ignoreMatcher := ignore.GetMatcher(r.Config)

	if err := paths.WalkDir(".", func(sysPath paths.System, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		gitPath := sysPath.ToGit()

		isDir := d.IsDir()
		if ignoreMatcher.Match(gitPath, isDir) {
			// excluded from ignore
			if isDir {
				return filepath.SkipDir
			} else {
				return nil
			}
		}
		if isDir {
			return nil
		}

		if err := r.PushFile(sysPath); err != nil {
			return errors.Join(fmt.Errorf("failed to push file %s to remote", sysPath), err)
		}

		return nil
	}); err != nil {
		return errors.Join(errors.New("failed to walk repo dir"), err)
	}

	return nil
}

func (r *Remote) CommitInteractive() error {
	if err := r.PushIgnore(); err != nil {
		return errors.Join(errors.New("failed to push ignore"), err)
	}

	commitables, err := r.getCommitable()
	if err != nil {
		return errors.Join(errors.New("failed to get local changes"), err)
	}

	for _, cf := range commitables {
		fmt.Printf("%s %s\n", cf.Status.ToString(), cf.Path)
	}

	return nil
}

func (r *Remote) getCommitable() ([]commitFile, error) {
	ignoreMatcher := ignore.GetMatcher(r.Config)

	var (
		commitables   []commitFile
		existingFiles []paths.Unix
	)

	if options.FlagVerbose {
		fmt.Println("checking local files for changes")
	}

	if err := paths.WalkDir(".", func(sysPath paths.System, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		gitPath := sysPath.ToGit()

		isDir := d.IsDir()
		if ignoreMatcher.Match(gitPath, isDir) {
			// excluded from ignore
			if isDir {
				return filepath.SkipDir
			} else {
				return nil
			}
		}
		if isDir {
			return nil
		}

		unixPath := sysPath.ToUnix()
		existingFiles = append(existingFiles, unixPath)

		if exists, err := r.existsOnRemote(unixPath); err != nil {
			return errors.Join(fmt.Errorf("failed to check if file %s exists on remote", sysPath), err)
		} else if !exists {
			commitables = append(commitables, commitFile{
				Path:   sysPath,
				Status: commitFileStatusCreate,
			})
			return nil
		}

		rh, err := r.getRemoteHash(unixPath)
		if err != nil {
			return errors.Join(fmt.Errorf("failed to get remote hash from %s", unixPath), err)
		}
		lh, err := sysPath.Hash()
		if err != nil {
			return errors.Join(fmt.Errorf("failed to get hash from %s", sysPath), err)
		}
		if bytes.Equal(rh, lh) {
			return nil
		}
		commitables = append(commitables, commitFile{
			Path:   sysPath,
			Status: commitFileStatusChange,
		})

		return nil
	}); err != nil {
		return nil, errors.Join(errors.New("failed to walk repo dir"), err)
	}

	if options.FlagVerbose {
		fmt.Println("checking remote files for deletes")
	}

	remoteWalkRoot := path.Join(r.Config.Remote.Path, DirHash)
	remoteWalker := r.SftpClient.Walk(remoteWalkRoot)
	for remoteWalker.Step() {
		unixPath, err := paths.Unix(remoteWalker.Path()).Rel(remoteWalkRoot)
		if err != nil {
			return nil, errors.Join(errors.New("failed to walk remote file system"), err)
		}
		gitPath := unixPath.ToGit()

		isDir := remoteWalker.Stat().IsDir()
		if ignoreMatcher.Match(gitPath, isDir) {
			// excluded from ignore
			if isDir {
				remoteWalker.SkipDir()
			}
			continue
		}
		if isDir {
			continue
		}

		if slices.Index(existingFiles, unixPath) == -1 {
			commitables = append(commitables, commitFile{
				Path:   unixPath,
				Status: commitFileStatusDelete,
			})
		}
	}

	return commitables, nil
}

func (r *Remote) PushIgnore() error {
	if options.FlagVerbose {
		fmt.Print("Pushing ignore... ")
		defer fmt.Println()
	}

	remoteName := path.Join(r.Config.Remote.Path, FileIgnore)

	rf, err := r.SftpClient.Create(remoteName)
	if err != nil {
		return errors.Join(fmt.Errorf("failed to open file %s on remote", remoteName))
	}
	defer rf.Close()
	if _, err := rf.Write([]byte(r.Config.Ignore)); err != nil {
		return errors.Join(errors.New("failed to write ignore to remote"), err)
	}

	if options.FlagVerbose {
		fmt.Print("done")
	}

	return nil
}

func (r *Remote) existsOnRemote(name paths.Path) (bool, error) {
	unixNameStr := name.ToUnix().ToString()

	remoteName := path.Join(r.Config.Remote.Path, DirContent, unixNameStr+".gz")
	remoteHashName := path.Join(r.Config.Remote.Path, DirHash, unixNameStr)

	if _, err := r.SftpClient.Stat(remoteName); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, err
		}
	}

	if _, err := r.SftpClient.Stat(remoteHashName); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, err
		}
	}

	return true, nil
}

func (r *Remote) getRemoteHash(name paths.Path) ([]byte, error) {
	remoteHashName := path.Join(r.Config.Remote.Path, DirHash, name.ToUnix().ToString())
	f, err := r.SftpClient.Open(remoteHashName)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to open remote file %s", remoteHashName), err)
	}
	defer f.Close()

	h, err := io.ReadAll(f)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to read remote file %s", remoteHashName), err)
	}

	return h, nil
}

func (r *Remote) PushFile(sysPath paths.System) error {
	if options.FlagVerbose {
		fmt.Printf("pushing %s... ", sysPath)
		defer fmt.Println()
	}

	unixName := sysPath.ToUnix()

	remoteDir := path.Join(r.Config.Remote.Path, DirContent, path.Dir(string(unixName)))
	remoteHashDir := path.Join(r.Config.Remote.Path, DirHash, path.Dir(string(unixName)))
	remoteName := path.Join(r.Config.Remote.Path, DirContent, string(unixName)+".gz")
	remoteHashName := path.Join(r.Config.Remote.Path, DirHash, string(unixName))

	f, err := sysPath.Open()
	if err != nil {
		return errors.Join(fmt.Errorf("failed to open local file %s", sysPath), err)
	}
	defer f.Close()

	if err := r.SftpClient.MkdirAll(remoteDir); err != nil && !os.IsExist(err) {
		return errors.Join(fmt.Errorf("failed to make directory %s on remote", remoteDir))
	}
	if err := r.SftpClient.MkdirAll(remoteHashDir); err != nil && !os.IsExist(err) {
		return errors.Join(fmt.Errorf("failed to make directory %s on remote", remoteHashDir))
	}

	rf, err := r.SftpClient.Create(remoteName)
	if err != nil {
		return errors.Join(fmt.Errorf("failed to open file %s on remote", remoteName))
	}
	defer rf.Close()

	gw, err := gzip.NewWriterLevel(rf, gzip.BestCompression)
	if err != nil {
		return errors.Join(fmt.Errorf("failed to open gzip writer for %s on remote", remoteName))
	}
	defer gw.Close()

	h := sha256.New()

	mw := io.MultiWriter(h, gw)

	if _, err := io.Copy(mw, f); err != nil {
		return errors.Join(fmt.Errorf("failed to copy file %s to remote", sysPath), err)
	}

	// Close the gzip writer explicitly to ensure all data is flushed
	if err := gw.Close(); err != nil {
		return errors.Join(fmt.Errorf("failed to close gzip writer for %s", remoteName), err)
	}

	hashVal := h.Sum(nil)

	hashFile, err := r.SftpClient.Create(remoteHashName)
	if err != nil {
		return errors.Join(fmt.Errorf("failed to create hash file %s on remote", remoteHashName), err)
	}
	defer hashFile.Close()

	if _, err := hashFile.Write(hashVal); err != nil {
		return errors.Join(fmt.Errorf("failed to write hash to file %s on remote", remoteHashName), err)
	}

	if options.FlagVerbose {
		fmt.Print("done")
	}

	return nil
}
