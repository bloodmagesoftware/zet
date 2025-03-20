package remote

import (
	"errors"
	"os"

	"github.com/bloodmagesoftware/zet/internal/project"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Remote struct {
	SshClient  *ssh.Client
	SftpClient *sftp.Client
	Config     project.Project
}

func (r *Remote) Close() error {
	if r == nil {
		return nil
	}

	if r.SftpClient != nil {
		if err := r.SftpClient.Close(); err != nil {
			_ = r.SshClient.Close()
			return errors.Join(errors.New("failed to close SFTP client"), err)
		}
		r.SftpClient = nil
	}

	if r.SshClient != nil {
		if err := r.SshClient.Close(); err != nil {
			return errors.Join(errors.New("failed to close SSH client"), err)
		}
		r.SshClient = nil
	}

	return nil
}

func Connect(p project.Project) (*Remote, error) {
	r := &Remote{Config: p}
	var err error

	r.SshClient, err = connectSsh(p)
	if err != nil {
		return nil, errors.Join(errors.New("failed to establish ssh connection"), err)
	}

	r.SftpClient, err = sftp.NewClient(r.SshClient)
	if err != nil {
		return nil, errors.Join(errors.New("failed to establish sftp connection"), err)
	}

	if err := r.SftpClient.MkdirAll(p.Remote.Path); err != nil && !os.IsExist(err) {
		return nil, errors.Join(errors.New("failed to make remote directory"), err)
	}

	return r, nil
}
