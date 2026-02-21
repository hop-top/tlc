package core

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

func GetCurrentUser() string {
	if user := os.Getenv("TLC_USER"); user != "" {
		return user
	}
	cmd := exec.CommandContext(context.Background(), "git", "config", "user.name")
	if out, err := cmd.Output(); err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			return name
		}
	}
	return os.Getenv("USER")
}
