package remote

import (
	"fmt"
	"net"
	"os/user"
	"path/filepath"

	"github.com/bloodmagesoftware/zet/internal/options"
	"github.com/bloodmagesoftware/zet/internal/project"
	"github.com/bloodmagesoftware/zet/internal/util"
	"github.com/charmbracelet/huh"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func hostKeyCallback() ssh.HostKeyCallback {
	if u, err := user.Current(); err == nil {
		path := filepath.Join(u.HomeDir, ".ssh", "known_hosts")
		if util.Exists(path) {
			if hostKeyCallback, err := knownhosts.New(path); err == nil {
				return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
					if options.FlagForce {
						return nil
					}
					ok := false
					if err := hostKeyCallback(hostname, remote, key); err != nil {
						if fErr := huh.NewForm(huh.NewGroup(
							huh.NewConfirm().
								Title(err.Error()).
								Value(&ok).
								Affirmative("Allow").
								Negative("Cancel"),
						)).Run(); fErr == nil && ok {
							return nil
						}
						return err
					} else {
						return nil
					}
				}
			}
		}
	}
	return ssh.InsecureIgnoreHostKey()
}

func connectSsh(p project.Project) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User: p.Remote.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(p.Remote.Password),
		},
		HostKeyCallback: hostKeyCallback(),
	}

	addr := fmt.Sprintf("%s:%d", p.Remote.Hostname, p.Remote.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, err
	}

	return client, nil
}
