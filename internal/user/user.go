package user

import (
	"fmt"
	"os/user"
)

var u *user.User

func Name() string {
	if u == nil {
		var err error
		u, err = user.Current()
		if err != nil {
			panic(fmt.Sprintf("failed to get user: %s", err.Error()))
		}
	}
	return u.Username
}
