package cli

import (
	"errors"
	"os"

	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

var GlobalDebug bool

type Root struct {
	Debug    bool             `short:"d" long:"debug" description:"Enable debug logging"`
	SSH      *SSHCommand      `command:"ssh"`
	SSHProxy *SSHProxyCommand `command:"ssh-proxy"`
}

func Execute() {
	var root Root
	parser := flags.NewParser(&root, flags.Default)

	parser.CommandHandler = func(command flags.Commander, args []string) error {
		GlobalDebug = root.Debug

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
