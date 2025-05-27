package ssh_proxy

import (
	"context"

	awsUtils "aws-ec2-ssh/internal/awsutils"
	sshUtils "aws-ec2-ssh/internal/sshutils"
	"github.com/aws/aws-sdk-go-v2/aws"
	ec2InstanceConnect "github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	ssmClient "github.com/mmmorris1975/ssm-session-client/ssmclient"
	log "github.com/sirupsen/logrus"
)

type Command struct {
	Profile    string `short:"p" long:"profile" description:"AWS CLI profile to use" env:"AWS_PROFILE"`
	Region     string `short:"r" long:"region" description:"AWS region (defaults to profile region)" env:"AWS_REGION"`
	Username   string `short:"u" long:"username" description:"SSH username"`
	InstanceId string `short:"i" long:"instance-id" description:"EC2 Instance ID"`
	Port       int    `short:"P" long:"port" description:"SSH port" default:"22"`
	PublicKey  string `short:"k" long:"ssh-key" description:"SSH private key path (default: first found in ~/.ssh/)"`
}

func (c *Command) Execute(args []string) error {
	cfg := awsUtils.LoadAwsDefaultConfig(c.Profile, c.Region)

	log.Debugf("Reading SSH private key: %s\n", c.PublicKey)

	publicKey, err := sshUtils.ReadPublicKey(c.PublicKey)
	if err != nil {
		log.Fatal(err)
	}

	ec2ic := ec2InstanceConnect.NewFromConfig(cfg)

	log.Debugf("Sending SSH public key %s for username: %s instance ID: %s\n", c.PublicKey, c.Username, c.InstanceId)

	if _, err = ec2ic.SendSSHPublicKey(context.Background(), &ec2InstanceConnect.SendSSHPublicKeyInput{
		InstanceId:     aws.String(c.InstanceId),
		InstanceOSUser: aws.String(c.Username),
		SSHPublicKey:   aws.String(publicKey),
	}); err != nil {
		log.Fatal(err)
	}

	log.Debugln("Starting SSH session")

	err = ssmClient.SSHSession(cfg, &ssmClient.PortForwardingInput{
		Target:     c.InstanceId,
		RemotePort: c.Port,
	})
	if err != nil {
		log.Fatal(err)
	}

	return nil
}
