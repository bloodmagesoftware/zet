package remote

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"time"

	"github.com/bloodmagesoftware/zet/internal/ignore"
	"github.com/bloodmagesoftware/zet/internal/options"
	"github.com/bloodmagesoftware/zet/internal/paths"
	"github.com/bloodmagesoftware/zet/internal/user"
	"github.com/charmbracelet/huh"
)

const (
	DirContent = "content"
	DirMeta    = "meta"
	FileIgnore = "ignore"
)

type Meta struct {
	Hash       []byte    `json:"hash"`
	LastEditor string    `json:"last_editor"`
	LastEdit   time.Time `json:"last_edit"`
}

type (
	commitFile struct {
		Path       paths.Path
		Status     commitFileStatus
		LastEditor string
		LastEdit   time.Time
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
		return "ADD   "
	case commitFileStatusDelete:
		return "DELETE"
	case commitFileStatusChange:
		return "CHANGE"
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
		fmt.Println("initial commit")
	}

	if err := r.pushIgnore(); err != nil {
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

		if err := r.pushFile(sysPath); err != nil {
			return errors.Join(fmt.Errorf("failed to push file %s to remote", sysPath), err)
		}

		return nil
	}); err != nil {
		return errors.Join(errors.New("failed to walk repo dir"), err)
	}

	return nil
}

func (r *Remote) CommitInteractive() error {
	if err := r.pushIgnore(); err != nil {
		return errors.Join(errors.New("failed to push ignore"), err)
	}

	commitables, err := r.getCommitable()
	if err != nil {
		return errors.Join(errors.New("failed to get local changes"), err)
	}

	opts := make([]huh.Option[*commitFile], len(commitables))

	for i, cf := range commitables {
		if cf.Status == commitFileStatusCreate {
			opts[i] = huh.Option[*commitFile]{
				Key:   fmt.Sprintf("%s %s", cf.Status.ToString(), cf.Path.ToString()),
				Value: &cf,
			}
		} else {
			opts[i] = huh.Option[*commitFile]{
				Key:   fmt.Sprintf("%s %s previously changed at %s by %s", cf.Status.ToString(), cf.Path.ToString(), cf.LastEdit.Format(time.UnixDate), cf.LastEditor),
				Value: &cf,
			}
		}
	}

	var selectedCommitables []*commitFile

	if err := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[*commitFile]().
			Title("Diff from current remote").
			Options(opts...).
			Value(&selectedCommitables),
	)).Run(); err != nil {
		return err
	}

	for _, cf := range selectedCommitables {
		switch cf.Status {
		case commitFileStatusCreate:
			if err := r.pushFile(cf.Path.(paths.System)); err != nil {
				return errors.Join(fmt.Errorf("failed to create %s", cf.Path.ToString()), err)
			}
		case commitFileStatusDelete:
			if err := r.removeFile(cf.Path); err != nil {
				return errors.Join(fmt.Errorf("failed to delete %s", cf.Path.ToString()), err)
			}
		case commitFileStatusChange:
			if err := r.pushFile(cf.Path.(paths.System)); err != nil {
				return errors.Join(fmt.Errorf("failed to change %s", cf.Path.ToString()), err)
			}
		}
	}

	return nil
}

func (r *Remote) getCommitable() ([]commitFile, error) {
	ignoreMatcher := ignore.GetMatcher(r.Config)
	now := time.Now()

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
				sysPath,
				commitFileStatusCreate,
				"",
				now,
			})
			return nil
		}

		rm, err := r.getRemoteMeta(unixPath)
		if err != nil {
			return errors.Join(fmt.Errorf("failed to get remote meta from %s", unixPath), err)
		}
		lh, err := sysPath.Hash()
		if err != nil {
			return errors.Join(fmt.Errorf("failed to get hash from %s", sysPath), err)
		}
		if bytes.Equal(rm.Hash, lh) {
			return nil
		}
		commitables = append(commitables, commitFile{
			sysPath,
			commitFileStatusChange,
			rm.LastEditor,
			rm.LastEdit,
		})

		return nil
	}); err != nil {
		return nil, errors.Join(errors.New("failed to walk repo dir"), err)
	}

	if options.FlagVerbose {
		fmt.Println("checking remote files for deletes")
	}

	remoteWalkRoot := path.Join(r.Config.Remote.Path, DirMeta)
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

		rf, err := r.SftpClient.Open(remoteWalker.Path())
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to open remote file %s", remoteWalker.Path()), err)
		}
		defer rf.Close()
		rm := Meta{}
		if json.NewDecoder(rf).Decode(&rm); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to get remote meta from %s", unixPath), err)
		}

		if slices.Index(existingFiles, unixPath) == -1 {
			commitables = append(commitables, commitFile{
				unixPath,
				commitFileStatusDelete,
				rm.LastEditor,
				rm.LastEdit,
			})
		}
	}

	return commitables, nil
}

func (r *Remote) pushIgnore() error {
	if options.FlagVerbose {
		fmt.Print("pushing ignore... ")
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
	remoteMetaName := path.Join(r.Config.Remote.Path, DirMeta, unixNameStr)

	if _, err := r.SftpClient.Stat(remoteName); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, err
		}
	}

	if _, err := r.SftpClient.Stat(remoteMetaName); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, err
		}
	}

	return true, nil
}

func (r *Remote) getRemoteMeta(name paths.Path) (Meta, error) {
	m := Meta{}
	remoteMetaName := path.Join(r.Config.Remote.Path, DirMeta, name.ToUnix().ToString())
	f, err := r.SftpClient.Open(remoteMetaName)
	if err != nil {
		return m, errors.Join(fmt.Errorf("failed to open remote file %s", remoteMetaName), err)
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return m, errors.Join(fmt.Errorf("failed to read remote file %s", remoteMetaName), err)
	}

	return m, nil
}

func (r *Remote) removeFile(pat paths.Path) error {
	if options.FlagVerbose {
		fmt.Printf("removing %s... ", pat)
		defer fmt.Println()
	}

	unixName := pat.ToUnix()
	remoteName := path.Join(r.Config.Remote.Path, DirContent, string(unixName)+".gz")
	remoteMetaName := path.Join(r.Config.Remote.Path, DirMeta, string(unixName))

	if err := r.SftpClient.Remove(remoteMetaName); err != nil {
		return errors.Join(fmt.Errorf("failed to remove file %s", remoteMetaName), err)
	}
	if err := r.SftpClient.Remove(remoteName); err != nil {
		return errors.Join(fmt.Errorf("failed to remove file %s", remoteName), err)
	}
	return nil
}

func (r *Remote) pushFile(pat paths.System) error {
	if options.FlagVerbose {
		fmt.Printf("pushing %s... ", pat)
		defer fmt.Println()
	}

	unixName := pat.ToUnix()

	remoteDir := path.Join(r.Config.Remote.Path, DirContent, path.Dir(string(unixName)))
	remoteMetaDir := path.Join(r.Config.Remote.Path, DirMeta, path.Dir(string(unixName)))
	remoteName := path.Join(r.Config.Remote.Path, DirContent, string(unixName)+".gz")
	remoteMetaName := path.Join(r.Config.Remote.Path, DirMeta, string(unixName))

	stat, err := pat.Stat()
	if err != nil {
		return errors.Join(fmt.Errorf("failed to stat file %s", pat), err)
	}

	f, err := pat.Open()
	if err != nil {
		return errors.Join(fmt.Errorf("failed to open local file %s", pat), err)
	}
	defer f.Close()

	if err := r.SftpClient.MkdirAll(remoteDir); err != nil && !os.IsExist(err) {
		return errors.Join(fmt.Errorf("failed to make directory %s on remote", remoteDir))
	}
	if err := r.SftpClient.MkdirAll(remoteMetaDir); err != nil && !os.IsExist(err) {
		return errors.Join(fmt.Errorf("failed to make directory %s on remote", remoteMetaDir))
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
		return errors.Join(fmt.Errorf("failed to copy file %s to remote", pat), err)
	}

	// Close the gzip writer explicitly to ensure all data is flushed
	if err := gw.Close(); err != nil {
		return errors.Join(fmt.Errorf("failed to close gzip writer for %s", remoteName), err)
	}

	hashVal := h.Sum(nil)

	metaFile, err := r.SftpClient.Create(remoteMetaName)
	if err != nil {
		return errors.Join(fmt.Errorf("failed to create meta file %s on remote", remoteMetaName), err)
	}
	defer metaFile.Close()

	m := Meta{
		hashVal,
		user.Name(),
		stat.ModTime(),
	}
	if err := json.NewEncoder(metaFile).Encode(&m); err != nil {
		return errors.Join(fmt.Errorf("failed to write meta to file %s on remote", remoteMetaName), err)
	}

	if options.FlagVerbose {
		fmt.Print("done")
	}

	return nil
}
