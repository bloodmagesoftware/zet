package project

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	ignore_templates "github.com/bloodmagesoftware/zet/internal/ignore/templates"
	"github.com/charmbracelet/huh"
	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

type (
	Project struct {
		Version int    `json:"version"`
		Remote  Remote `json:"remote"`
		Ignore  string `json:"ignore"`
	}

	Remote struct {
		Hostname string `json:"hostname"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"-"`
		Path     string `json:"path"`
	}
)

const (
	KeyringService  = "de.bloodmagesoftware.zet"
	ProjectFileName = ".zet.yaml"
	Version         = 1
)

func Exists() (bool, error) {
	stat, err := os.Stat(ProjectFileName)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if stat.IsDir() {
		return true, errors.New("project file is a directory")
	}
	return true, nil
}

func Load() (Project, error) {
	p := Project{Remote: Remote{}}

	f, err := os.Open(ProjectFileName)
	if err != nil {
		return p, errors.Join(errors.New("failed to open project file"), err)
	}
	defer f.Close()

	yd := yaml.NewDecoder(f)
	if err := yd.Decode(&p); err != nil {
		return p, errors.Join(errors.New("failed to decode project file"), err)
	}

	if p.Remote.Hostname == "" {
		return Project{}, errors.Join(errors.New("unexpected error during project file decoding"), err)
	}

	{
		var err error
		if p.Remote.Password, err = keyring.Get(KeyringService, p.UserString()); err != nil {
			return p, errors.Join(fmt.Errorf("failed to get keyring credentials for user %s", p.UserString()), err)
		}
	}

	return p, nil
}

func NewInteractive() (Project, error) {
	p := Project{Version: Version, Remote: Remote{}}
	port := "22"
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Host").
			Value(&p.Remote.Hostname),
		huh.NewInput().
			Title("Port").
			Validate(func(s string) error {
				i, err := strconv.Atoi(s)
				if err != nil {
					return err
				}
				if i < 0 || i > 65535 {
					return errors.New("out of range 0-65535")
				}
				return nil
			}).
			Value(&port),
		huh.NewInput().
			Title("Username").
			Value(&p.Remote.Username),
		huh.NewInput().
			Title("Password").
			EchoMode(huh.EchoModePassword).
			Value(&p.Remote.Password),
		huh.NewInput().
			Title("Path").
			Value(&p.Remote.Path),
		huh.NewSelect[string]().
			Title("Ignore template").
			Value(&p.Ignore).
			Options(
				huh.Option[string]{Key: "Default", Value: ignore_templates.Default},
				huh.Option[string]{Key: "Unreal", Value: ignore_templates.Unreal},
				huh.Option[string]{Key: "Godot", Value: ignore_templates.Godot},
				huh.Option[string]{Key: "Bevy", Value: ignore_templates.Bevy},
			),
	)).Run(); err != nil {
		return p, err
	}

	var err error
	p.Remote.Port, err = strconv.Atoi(port)
	if err != nil {
		return p, errors.Join(fmt.Errorf("failed to parse port string %s to int", port), err)
	}

	if err := keyring.Set(
		KeyringService,
		p.UserString(),
		p.Remote.Password,
	); err != nil {
		return p, errors.Join(errors.New("failed to set keyring credentials"), err)
	}

	return p, nil
}

func (p Project) UserString() string {
	return fmt.Sprintf("%s@%s:%d", p.Remote.Username, p.Remote.Hostname, p.Remote.Port)
}

func Save(p Project) error {
	f, err := os.Create(ProjectFileName)
	if err != nil {
		return errors.Join(errors.New("failed to open project file"), err)
	}
	defer f.Close()

	ye := yaml.NewEncoder(f)
	defer ye.Close()
	ye.SetIndent(4)
	err = ye.Encode(&p)
	if err != nil {
		return errors.Join(errors.New("failed to encode project file"), err)
	}

	return nil
}
