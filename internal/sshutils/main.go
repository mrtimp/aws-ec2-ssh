package sshutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var defaultPrivateKeyFiles = []string{
	"~/.ssh/id_rsa",
	"~/.ssh/id_ecdsa",
	"~/.ssh/id_ecdsa_sk",
	"~/.ssh/id_ed25519",
	"~/.ssh/id_ed25519_sk",
}

func FindDefaultSSHPrivateKey() (string, error) {
	for _, k := range defaultPrivateKeyFiles {
		path := ExpandTilde(k)
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			return path, nil
		}
	}

	return "", fmt.Errorf("unable to find default SSH private key")
}

func ReadPublicKey(pubKeyPath string) (string, error) {
	if strings.HasPrefix(pubKeyPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		pubKeyPath = filepath.Join(home, pubKeyPath[2:])
	}

	data, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

func ExpandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()

		return filepath.Join(home, path[2:])
	}

	return path
}
