package root

import (
	"errors"
	"os"

	"aws-ec2-ssh/internal/cli/ssh"
	sshProxy "aws-ec2-ssh/internal/cli/ssh_proxy"
	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

type Root struct {
	Debug    bool              `short:"d" long:"debug" description:"Enable debug logging"`
	SSH      *ssh.Command      `command:"ssh"`
	SSHProxy *sshProxy.Command `command:"ssh-proxy"`
}

func Execute() {
	var root Root
	parser := flags.NewParser(&root, flags.Default)

	parser.CommandHandler = func(command flags.Commander, args []string) error {
		log.SetOutput(os.Stderr)

		if root.Debug {
			log.SetLevel(log.DebugLevel)
		} else {
			log.SetLevel(log.WarnLevel)
		}

		return command.Execute(args)
	}

	_, err := parser.Parse()
	if err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && errors.Is(flagsErr.Type, flags.ErrHelp) {
			os.Exit(0)
		}

		os.Exit(1)
	}
}
