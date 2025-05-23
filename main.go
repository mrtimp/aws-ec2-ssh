package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/smithy-go"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

type Options struct {
	Profile   string `short:"p" long:"profile" description:"AWS CLI profile to use"`
	Region    string `short:"r" long:"region" description:"AWS region (defaults to profile region)"`
	PublicKey string `short:"k" long:"key" description:"SSH private key path (default: first found in ~/.ssh/)"`
	Port      int    `short:"P" long:"port" description:"SSH port" default:"22"`
	Verbose   []bool `short:"v" description:"Increase SSH verbosity (use: -v, -vv, -vvv)"`
	Debug     bool   `short:"d" long:"debug" description:"Enable debug logging"`
	Args      struct {
		Instance string `positional-arg-name:"[user@]instance-id-or-name"`
	} `positional-args:"yes" required:"yes"`
}

var opts Options

var defaultUser = "ec2-user"

var defaultPrivateKeyFiles = []string{
	"~/.ssh/id_rsa",
	"~/.ssh/id_ecdsa",
	"~/.ssh/id_ecdsa_sk",
	"~/.ssh/id_ed25519",
	"~/.ssh/id_ed25519_sk",
}

func main() {
	_, err := flags.NewParser(&opts, flags.Default).Parse()
	if err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && errors.Is(flagsErr.Type, flags.ErrHelp) {
			os.Exit(0)
		}

		os.Exit(1)
	}

	log.SetOutput(os.Stderr)
	if opts.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}

	// find the users key in descending order if they didn't provide it
	publicKey := opts.PublicKey
	if publicKey == "" {
		publicKey, err = findDefaultSSHPrivateKey()
		if err != nil {
			log.Fatalf("SSH private key was not found in ~/.ssh (tried id_rsa, id_ecdsa, id_ecdsa_sk, id_ed25519, id_ed25519_sk)\n")
		}

		log.Debugf("Using automatically detected private key: %s", publicKey)
	}

	user := defaultUser
	instanceArg := opts.Args.Instance
	publicKey = expandTilde(publicKey)

	log.Debugf("Parsed flags: profile=%s, region=%s, user=%s, publicKey=%s, sshPort=%d, debug=%v, instanceArg=%s",
		opts.Profile, opts.Region, user, publicKey, opts.Port, opts.Debug, instanceArg)

	// split user @ instance
	if strings.Contains(instanceArg, "@") {
		parts := strings.SplitN(instanceArg, "@", 2)
		user = parts[0]
		instanceArg = parts[1]

		log.Debugf("Split username from instance: user=%s, instance=%s", user, instanceArg)
	}

	// if the instance ID doesn't match i-xxxx or mi-xxx then attempt
	// to resolve the instance ID from a potential name tag
	var instanceIDPattern = regexp.MustCompile(`^m?i-[[:xdigit:]]{8,}$`)

	if !instanceIDPattern.Match([]byte(instanceArg)) {
		resolved, err := resolveNameTagToInstanceID(instanceArg, opts.Profile, opts.Region)
		if err != nil {
			log.Fatalf("Error: %v\n", err)
		}

		log.Debugf("Resolved name '%s' to instance ID: %s", instanceArg, resolved)
		instanceArg = resolved
	}

	publicKeyPath := publicKey + ".pub"
	if _, err := os.Stat(publicKeyPath); err != nil {
		log.Fatalf("Could not find matching public key for %s (expected %s)\n", publicKey, publicKeyPath)
	}

	proxyCmd := fmt.Sprintf(
		"aws ec2-instance-connect send-ssh-public-key --instance-id %%h --instance-os-user %%r --ssh-public-key file://%s%s%s > /dev/null && aws ssm start-session --target %%h --document-name AWS-StartSSHSession --parameters portNumber=%%p%s%s",
		publicKeyPath,
		profileArg(opts.Profile),
		regionArg(opts.Region),
		profileArg(opts.Profile),
		regionArg(opts.Region),
	)
	log.Debugf("ProxyCommand: %s", proxyCmd)

	sshArgs := []string{
		"-i", publicKey,
		"-o", fmt.Sprintf("ProxyCommand=sh -c '%s'", proxyCmd),
		"-p", fmt.Sprint(opts.Port),
	}

	if len(opts.Verbose) > 0 {
		sshVerbosity := fmt.Sprintf("-%s", strings.Repeat("v", len(opts.Verbose)))
		sshArgs = append(sshArgs, sshVerbosity)
	}

	sshArgs = append(sshArgs, fmt.Sprintf("%s@%s", user, instanceArg))

	log.Debugf("SSH command: ssh %s", strings.Join(sshArgs, " "))

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("ssh failed: %v", err)
	}
}

func findDefaultSSHPrivateKey() (string, error) {
	for _, k := range defaultPrivateKeyFiles {
		path := expandTilde(k)
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			return path, nil
		}
	}

	return "", fmt.Errorf("no default SSH private key found")
}

func resolveNameTagToInstanceID(name, profile, region string) (string, error) {
	var opts []func(*config.LoadOptions) error
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}

	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	log.Debugf("Loading AWS config: profile=%s, region=%s", profile, region)

	cfg, err := config.LoadDefaultConfig(context.TODO(), opts...)
	if err != nil {
		return "", err
	}

	client := ec2.NewFromConfig(cfg)

	// filter running instances by Name tag
	out, err := client.DescribeInstances(context.TODO(), &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: []string{"running"},
			},
			{
				Name:   aws.String("tag:Name"),
				Values: []string{name},
			},
		},
	})
	if err != nil {
		log.Fatalf("Error describing instances: %v", err)
	}

	var matches []string
	for _, reservation := range out.Reservations {
		for _, instance := range reservation.Instances {
			matches = append(matches, aws.ToString(instance.InstanceId))
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("there are no running instance with Name tag: %s", name)
	}

	if len(matches) > 1 {
		return "", fmt.Errorf("there are multiple running instances with Name tag '%s': %v", name, matches)
	}

	return matches[0], nil
}

func profileArg(profile string) string {
	if profile == "" {
		return ""
	}

	return fmt.Sprintf(" --profile %s", profile)
}

func regionArg(region string) string {
	if region == "" {
		return ""
	}

	return fmt.Sprintf(" --region %s", region)
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()

		return filepath.Join(home, path[2:])
	}

	return path
}

func HandleAWSError(err error, profile string) bool {
	var ae smithy.APIError

	if errors.As(err, &ae) {
		code := ae.ErrorCode()
		msg := ae.ErrorMessage()
		switch code {
		case "ExpiredToken", "InvalidClientTokenId":
			log.Errorf("Your AWS credentials have expired or are invalid.")
			log.Errorf("Run: aws sso login --profile %s", profile)
			return true
		case "AccessDenied", "UnauthorizedOperation", "UnrecognizedClientException":
			log.Errorf("Access denied: %s", msg)
			log.Errorf("Check your IAM permissions or AWS account. (profile: %s)", profile)
			return true
		case "RequestCanceled":
			log.Errorf("AWS request was canceled (possible credential/config timeout).")
			return true
		default:
			log.Errorf("AWS error (%s): %s", code, msg)
			return true
		}
	}

	// SSO expiration and other common string patterns
	if err != nil {
		msg := err.Error()

		switch {
		case strings.Contains(msg, "the SSO session has expired or is invalid"),
			strings.Contains(msg, "The SSO session associated with this profile has expired"),
			strings.Contains(msg, "SSO session token is invalid"):
			log.Errorf("Your AWS SSO session has expired or is invalid.")
			log.Errorf("Run: aws sso login --profile %s", profile)
			return true
		case strings.Contains(msg, "Unable to locate credentials"),
			strings.Contains(msg, "NoCredentialProviders"):
			log.Errorf("No AWS credentials found for profile '%s'.", profile)
			log.Errorf("Configure credentials with: aws configure --profile %s", profile)
			return true
		}

		// Fallback: print the raw error (for less common cases)
		log.Errorf("Error: %v", err)
		return true
	}

	return false
}
