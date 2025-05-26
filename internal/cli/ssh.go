package cli

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"aws-ec2-ssh/internal/awsutils"
	"aws-ec2-ssh/internal/sshutils"
	log "github.com/sirupsen/logrus"
)

var defaultUser = "ec2-user"

type SSHCommand struct {
	Profile   string `short:"p" long:"profile" description:"AWS CLI profile to use" env:"AWS_PROFILE"`
	Region    string `short:"r" long:"region" description:"AWS region (defaults to profile region)" env:"AWS_REGION"`
	PublicKey string `short:"k" long:"key" description:"SSH private key path (default: first found in ~/.ssh/)"`
	Port      int    `short:"P" long:"port" description:"SSH port" default:"22"`
	Verbose   []bool `short:"v" description:"Increase SSH verbosity (use: -v, -vv, -vvv)"`
	Args      struct {
		Instance string `positional-arg-name:"[user@]instance-id-or-name"`
	} `positional-args:"yes" required:"yes"`
}

func (c *SSHCommand) Execute(args []string) error {
	var err error

	// find the users key in descending order if they didn't provide it
	publicKey := c.PublicKey
	if publicKey == "" {
		publicKey, err = sshutils.FindDefaultSSHPrivateKey()
		if err != nil {
			log.Fatalf("SSH private key was not found in ~/.ssh (tried id_rsa, id_ecdsa, id_ecdsa_sk, id_ed25519, id_ed25519_sk)\n")
		}

		log.Debugf("Using automatically detected private key: %s", publicKey)
	}

	user := defaultUser
	instanceArg := c.Args.Instance
	instanceId := instanceArg
	publicKey = sshutils.ExpandTilde(publicKey)

	log.Debugf("Parsed flags: profile=%s, region=%s, user=%s, publicKey=%s, sshPort=%d, debug=%v, instanceArg=%s",
		c.Profile, c.Region, user, publicKey, c.Port, GlobalDebug, instanceArg)

	// split user @ instance
	if strings.Contains(instanceArg, "@") {
		parts := strings.SplitN(instanceArg, "@", 2)
		user = parts[0]
		instanceId = parts[1]

		log.Debugf("Split username from instance: user=%s, instance=%s", user, instanceId)
	}

	// if the instance ID doesn't match i-xxxx or mi-xxx then attempt
	// to resolve the instance ID from a potential name tag
	var instanceIDPattern = regexp.MustCompile(`^m?i-[[:xdigit:]]{8,}$`)

	if !instanceIDPattern.Match([]byte(instanceId)) {
		resolved, err := awsutils.ResolveNameTagToInstanceID(instanceId, c.Profile, c.Region)
		if err != nil {
			log.Fatalf("Error: %v\n", err)
		}

		log.Debugf("Resolved name '%s' to instance ID: %s", instanceId, resolved)
		instanceId = resolved
	}

	publicKeyPath := publicKey + ".pub"
	if _, err := os.Stat(publicKeyPath); err != nil {
		log.Fatalf("Could not find matching public key for %s (expected %s)\n", publicKey, publicKeyPath)
	}

	proxyCmd := strings.TrimSpace(fmt.Sprintf(
		"aws-ec2-ssh ssh-proxy --ssh-key %s --username %s --instance-id %s %s %s",
		publicKeyPath,
		user,
		instanceId,
		prepareArg("profile", c.Profile),
		prepareArg("region", c.Region),
	))

	log.Debugf("ProxyCommand: %s", proxyCmd)

	sshArgs := []string{
		"-i", publicKey,
		"-o", fmt.Sprintf("ProxyCommand=sh -c '%s'", proxyCmd),
		"-p", fmt.Sprint(c.Port),
	}

	if len(c.Verbose) > 0 {
		sshVerbosity := fmt.Sprintf("-%s", strings.Repeat("v", len(c.Verbose)))
		sshArgs = append(sshArgs, sshVerbosity)
	}

	sshArgs = append(sshArgs, fmt.Sprintf("%s@%s", user, instanceId))

	log.Debugf("SSH command: ssh %s", strings.Join(sshArgs, " "))

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("ssh failed: %v", err)
	}

	return nil
}

func prepareArg(arg string, profile string) string {
	if profile == "" {
		return ""
	}

	return fmt.Sprintf(" --%s %s", arg, profile)
}
