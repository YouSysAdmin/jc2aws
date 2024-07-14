package main

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/yousysadmin/jc2aws/internal/aws"
	"github.com/yousysadmin/jc2aws/internal/jumpcloud"
	"github.com/yousysadmin/jc2aws/internal/totp"
	"io"
	"os"
	"path/filepath"

	"github.com/yousysadmin/jc2aws/internal/config"
	"github.com/yousysadmin/jc2aws/pkg"
)

// UserHomeDir retrive current user home dir path
var UserHomeDir = func() string {
	path, _ := os.UserHomeDir()
	return path
}

type App struct {
	// Store user input for flags value
	Email          string // JumpCloud Email
	Password       string // JumpCloud Password
	MfaToken       string // JumpCloud MFA Token or MFA Secret
	IdpURL         string // JumpCloud IDP URL
	AccountName    string // AWS account name in config file (interactive and non-interactive mode)
	RoleARN        string // AWS role ARN
	PrincipalARN   string // AWS Principal ARN
	Region         string // AWS Region
	Duration       int    // Credential expiration duration
	ConfigFilePath string // Path to a config file
	Interactive    bool   // Interactive mode flag
	OutputFile     string // Credentials output file path
	OutputFormat   string // Credential output format
	AwsCliProfile  string // AWS CLI profile name (for `profile` output format)

	Config *config.Config
	Cli    *cli.App
}

func main() {
	app := App{
		Config:         &config.Config{},
		ConfigFilePath: filepath.Join(UserHomeDir(), config.DefaultConfigFileName),
		OutputFormat:   "cli", // cli, env, cli-stdout, env-stdout
		Interactive:    false,
		Duration:       3600,
	}

	app.cliInit()
	err := app.Cli.Run(os.Args)
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(1)
	}
}

// cliInit initialise CLI
func (app *App) cliInit() {
	app.Cli = &cli.App{
		Name:                 "",
		Usage:                "Get AWS credentials",
		UsageText:            "Get temporarily AWS credentials via Jumpcloud (SAML)",
		Version:              pkg.Version,
		EnableBashCompletion: true,
		Before: func(cCtx *cli.Context) error {
			cfgFile, err := config.NewConfig(app.ConfigFilePath)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(cCtx.App.Writer, "\n# **Warning:**\n# Config file %s not found\n\n", app.ConfigFilePath)
				return nil
			}
			app.Config = cfgFile

			if len(app.Config.Accounts) <= 0 {
				fmt.Fprintf(cCtx.App.Writer, "\n# **Warning:**\n# Not found any accounts in the config file %s\n\n", app.ConfigFilePath)
				return nil
			}

			return err
		},

		// CLI Commands
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "Path to a config file",
				EnvVars:     []string{"J2A_CONFIG"},
				Value:       app.ConfigFilePath,
				Destination: &app.ConfigFilePath,
			},
			&cli.BoolFlag{
				Name:        "interactive",
				Aliases:     []string{"i"},
				Usage:       "Turn on interactive mode",
				EnvVars:     []string{"J2A_INTERACTIVE"},
				Value:       app.Interactive,
				Destination: &app.Interactive,
			},
			&cli.StringFlag{
				Name:        "email",
				Aliases:     []string{"e"},
				Usage:       "Jumpcloud user email",
				EnvVars:     []string{"J2A_EMAIL"},
				Value:       app.Config.DefaultEmail,
				Destination: &app.Email,
			},
			&cli.StringFlag{
				Name:        "password",
				Aliases:     []string{"p"},
				Usage:       "Jumpcloud user password",
				EnvVars:     []string{"J2A_PASSWORD"},
				Value:       app.Config.DefaultPassword,
				Destination: &app.Password,
			},
			&cli.StringFlag{
				Name:        "mfa",
				Aliases:     []string{"m"},
				Usage:       "Jumpcloud user MFA token",
				EnvVars:     []string{"J2A_MFA"},
				Value:       app.Config.DefaultMFATokenSecret,
				Destination: &app.MfaToken,
			},
			&cli.StringFlag{
				Name:        "idp-url",
				Usage:       "Jumpcloud IDP URL (ex: https://sso.jumpcloud.com/saml2/my-aws-prod)",
				EnvVars:     []string{"J2A_IDP_URL"},
				Destination: &app.IdpURL,
			},
			&cli.StringFlag{
				Name:        "role-arn",
				Usage:       "AWS Role ARN (ex: arn:aws:iam::ACCOUNT-ID:role/admin)",
				EnvVars:     []string{"J2A_ROLE_ARN"},
				Destination: &app.RoleARN,
			},
			&cli.StringFlag{
				Name:        "principal-arn",
				Usage:       "AWS Identity provider ARN (ex: arn:aws:iam::ACCOUNT-ID:saml-provider/jumpcloud)",
				EnvVars:     []string{"J2A_PRINCIPAL_ARN"},
				Destination: &app.PrincipalARN,
			},
			&cli.StringFlag{
				Name:        "region",
				Aliases:     []string{"r"},
				Usage:       "AWS region (ex: us-west-2)",
				EnvVars:     []string{"J2A_AWS_REGION"},
				Destination: &app.Region,
			},
			&cli.IntFlag{
				Name:        "duration",
				Aliases:     []string{"d"},
				Usage:       "AWS credential expiration time",
				EnvVars:     []string{"J2A_DURATION"},
				Value:       app.Duration,
				Destination: &app.Duration,
			},
			&cli.StringFlag{
				Name:        "account",
				Aliases:     []string{"a"},
				Usage:       "Account name present in a config",
				EnvVars:     []string{"J2A_ACCOUNT"},
				Destination: &app.AccountName,
			},
			&cli.StringFlag{
				Name:        "output-format",
				Aliases:     []string{"f"},
				Usage:       "Credential output format (ex: cli, env, cli-stdout, env-stdout)",
				EnvVars:     []string{"J2A_OUTPUT_FORMAT"},
				Value:       app.OutputFormat,
				Destination: &app.OutputFormat,
			},
			&cli.StringFlag{
				Name:        "aws-cli-profile-name",
				Usage:       "AWS profile name used for store credentials",
				EnvVars:     []string{"J2A_AWS_CLI_PROFILE_NAME"},
				Value:       "default",
				Destination: &app.AwsCliProfile,
			},
		},
		Action: func(cCtx *cli.Context) error {
			// Validate request user input and validate input
			if err := promptOptions(app); err != nil {
				cli.ShowCommandHelp(cCtx, cCtx.Command.Name)
				return err
			}

			// Get credentials
			cred, err := getCredentials(app.Email, app.Password, app.IdpURL, app.MfaToken, app.PrincipalARN, app.RoleARN, app.Region, app.Duration)
			if err != nil {
				return err
			}

			// Output/Store credentials
			switch app.OutputFormat {
			case "cli": // store as aws-cli profile
				filePath := filepath.Join(UserHomeDir(), ".aws", "credentials")
				c, _ := cred.ToProfile(app.AwsCliProfile, filePath)
				if err := os.WriteFile(filePath, c, 0600); err != nil {
					return err
				}
			case "env": // store as env file
				filePath := filepath.Join(UserHomeDir(), ".jc2aws.env")
				c, _ := cred.ToEnv()
				if err := os.WriteFile(filePath, c, 0600); err != nil {
					return err
				}
			case "cli-stdout": // output to stdout as aws-cli profile
				c, _ := cred.ToProfile(app.AwsCliProfile, "")
				if _, err := io.WriteString(cCtx.App.Writer, string(c)); err != nil {
					return err
				}
			case "env-stdout": // output to stdout as env variables
				c, _ := cred.ToEnv()
				if _, err := io.WriteString(cCtx.App.Writer, string(c)); err != nil {
					return err
				}
			}

			return err
		},
	}
}

// promptOptions request input from a user and validate inputs
func promptOptions(app *App) (err error) {
	if len(app.Config.Accounts) > 0 {
		var account config.Account

		if app.Interactive && app.AccountName == "" {
			// Select account from a pre-configured account list in the interactive mode
			account, err = PromptAccount(app.Config.GetAccounts())
			if err != nil {
				return err
			}
		} else if (!app.Interactive && app.AccountName != "") || (app.Interactive && app.AccountName != "") {
			// Find account from a pre-configured account list by name in the non-interactive mode
			// or in the interactive mode with set account-name flag
			account, err = app.Config.FindAccountByName(app.AccountName)
			if err != nil {
				return err
			}
		}

		app.AccountName = account.Name
		app.Email = account.Email
		app.Password = account.Password
		app.IdpURL = account.IdpURL
		app.MfaToken = account.MFASecret
		app.PrincipalARN = account.AWSPrincipalArn
		app.Duration = account.Duration
		if app.AwsCliProfile == "" && account.AwsCliProfile == "" {
			app.AwsCliProfile = account.Name
		} else if app.AwsCliProfile == "" && account.AwsCliProfile != "" {
			app.AwsCliProfile = account.AwsCliProfile
		}

		// Select Role from a pre-configured account
		if len(account.AWSRoleArns) > 0 && app.RoleARN == "" && app.Interactive {
			app.RoleARN, err = PromptRoleArn(account)
			if err != nil {
				return err
			}
		}
		// Select AWS region from a pre-configured account
		if len(account.AWSRegions) > 0 && app.Region == "" && app.Interactive {
			app.Region, err = PromptRegion(account.AWSRegions)
			if err != nil {
				return err
			}
		}
	} else if len(app.Config.Accounts) <= 0 && app.AccountName != "" {
		return errors.New(AccountNameCantBeUsed)
	}

	if app.Email == "" {
		if app.Interactive {
			if app.Email, err = PromptSimple("Email", "email", "simple"); err != nil {
				return err
			}
		}

		return errors.New(EmailIsRequired)

	} else {
		if err = validators["email"](app.Email); err != nil {
			return err
		}
	}

	if app.Password == "" {
		if app.Interactive {
			if app.Password, err = PromptSimple("Password", "password", "masked"); err != nil {
				return err
			}
		}
		return errors.New(PasswordIsRequired)
	} else {
		if err = validators["password"](app.Password); err != nil {
			return err
		}
	}

	if app.IdpURL == "" {
		if app.Interactive {
			if app.IdpURL, err = PromptSimple("IDP URL", "idp-url", "simple"); err != nil {
				return err
			}
		}
		return errors.New(IdpURLRequred)
	} else {
		if err = validators["idp-url"](app.IdpURL); err != nil {
			return err
		}
	}

	if app.RoleARN == "" {
		if app.Interactive {
			if app.RoleARN, err = PromptSimple("Role ARN", "role-arn", "simple"); err != nil {
				return err
			}
		}
		return errors.New(AwsRoleArnIsRequired)
	} else {
		if err = validators["role-arn"](app.RoleARN); err != nil {
			return err
		}
	}

	if app.PrincipalARN == "" {
		if app.Interactive {
			if app.PrincipalARN, err = PromptSimple("Principal ARN", "principal-arn", "simple"); err != nil {
				return err
			}
		}
		return errors.New(AwsPrincipalUrlIsRequired)
	} else {
		if err = validators["principal-arn"](app.PrincipalARN); err != nil {
			return err
		}
	}

	if app.Region == "" {
		if app.Interactive {
			if app.Region, err = PromptRegion(aws.RegionsList); err != nil {
				return err
			}
		}
		return errors.New(AwsRegionIsRequired)
	} else {
		if err = validators["region"](app.Region); err != nil {
			return err
		}
	}

	if app.MfaToken == "" {
		if app.Interactive {
			if app.MfaToken, err = PromptSimple("MFA Token Or MFA Secret", "skip", "simple"); err != nil {
				return err
			}
		}
		return errors.New(MfaRequired)
	} else {
		if err = validators["mfa"](app.MfaToken); err != nil {
			return err
		}
	}

	if app.OutputFormat != "" {
		if err = validators["output-format"](app.OutputFormat); err != nil {
			return err
		}
	}

	return err
}

// getCredentials auth via Jumpcloud and get AWS credentials
func getCredentials(email, password, idpURL, mfa, principalARN, roleARN, region string, duration int) (cred aws.AwsSamlOutput, err error) {

	// If the length of the MFA parameter exceeds 6,
	// then it is an MFA secret and needs to obtain an MFA token based on it.
	if len(mfa) != 0 && len(mfa) > 6 {
		mfa = totp.GetToken(mfa)
	}

	jc, err := jumpcloud.New(email, password, idpURL, mfa)
	if err != nil {
		return cred, err
	}
	saml, err := jc.GetSaml()
	if err != nil {
		return cred, err
	}

	cred = aws.GetCredentials(aws.AwsSamlInput{
		PrincipalArn:    principalARN,
		RoleArn:         roleARN,
		SAMLAssertion:   saml,
		DurationSeconds: int32(duration),
		Region:          region,
	})

	return cred, err
}
