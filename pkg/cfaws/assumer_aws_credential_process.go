package cfaws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/processcreds"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/bigkevmcd/go-configparser"
)

// Implements Assumer using the aws credential_process standard
type CredentialProcessAssumer struct {
}

func loadCredProcessCreds(ctx context.Context, c *Profile) (aws.Credentials, error) {
	var credProcessCommand string
	for k, v := range c.RawConfig {
		if k == "credential_process" {
			credProcessCommand = v
			break
		}
	}
	p := processcreds.NewProvider(credProcessCommand)
	return p.Retrieve(ctx)
}

func (cpa *CredentialProcessAssumer) AssumeTerminal(ctx context.Context, c *Profile, configOpts ConfigOpts) (aws.Credentials, error) {
	// if the profile has parents, then we need to first use credentail process to assume the root profile.
	// then assume each of the chained profiles
	if len(c.Parents) != 0 {
		p := c.Parents[0]
		creds, err := loadCredProcessCreds(ctx, p)
		if err != nil {
			return creds, err
		}
		for _, p := range c.Parents[1:] {
			region, err := p.Region(ctx)
			if err != nil {
				return aws.Credentials{}, err
			}
			stsp := stscreds.NewAssumeRoleProvider(sts.New(sts.Options{Credentials: aws.NewCredentialsCache(&CredProv{creds}), Region: region}), p.AWSConfig.RoleARN, func(aro *stscreds.AssumeRoleOptions) {
				aro.RoleSessionName = sessionName()
				if p.AWSConfig.MFASerial != "" {
					aro.SerialNumber = &p.AWSConfig.MFASerial
					aro.TokenProvider = MfaTokenProvider
				} else if c.AWSConfig.MFASerial != "" {
					aro.SerialNumber = &c.AWSConfig.MFASerial
					aro.TokenProvider = MfaTokenProvider
				}
				aro.Duration = configOpts.Duration
			})
			creds, err = stsp.Retrieve(ctx)
			if err != nil {
				return creds, err
			}
		}
		region, err := c.Region(ctx)
		if err != nil {
			return aws.Credentials{}, err
		}
		stsp := stscreds.NewAssumeRoleProvider(sts.New(sts.Options{Credentials: aws.NewCredentialsCache(&CredProv{creds}), Region: region}), c.AWSConfig.RoleARN, func(aro *stscreds.AssumeRoleOptions) {
			aro.RoleSessionName = sessionName()
			if c.AWSConfig.MFASerial != "" {
				aro.SerialNumber = &c.AWSConfig.MFASerial
				aro.TokenProvider = MfaTokenProvider
			}
			aro.Duration = configOpts.Duration
		})
		return stsp.Retrieve(ctx)
	}

	return loadCredProcessCreds(ctx, c)

}

func (cpa *CredentialProcessAssumer) AssumeConsole(ctx context.Context, c *Profile, configOpts ConfigOpts) (aws.Credentials, error) {
	return cpa.AssumeTerminal(ctx, c, configOpts)
}

// A unique key which identifies this assumer e.g AWS-SSO or GOOGLE-AWS-AUTH
func (cpa *CredentialProcessAssumer) Type() string {
	return "AWS_CREDENTIAL_PROCESS"
}

// inspect for any credential processes with the saml2aws tool
func (cpa *CredentialProcessAssumer) ProfileMatchesType(rawProfile configparser.Dict, parsedProfile config.SharedConfig) bool {
	for k := range rawProfile {
		if k == "credential_process" {
			return true
		}
	}
	return false
}
