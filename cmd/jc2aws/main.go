package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"github.com/yousysadmin/jc2aws/internal/aws"
	"github.com/yousysadmin/jc2aws/internal/config"
	"github.com/yousysadmin/jc2aws/internal/jumpcloud"
	"github.com/yousysadmin/jc2aws/internal/totp"
	"github.com/yousysadmin/jc2aws/pkg"
)

// UserHomeDir retrieve current user home dir path
var UserHomeDir = func() string {
	path, _ := os.UserHomeDir()
	return path
}

type App struct {
	Email          string // JumpCloud Email
	Password       string // JumpCloud Password
	MfaToken       string // JumpCloud MFA Token or MFA Secret
	IdpURL         string // JumpCloud IDP URL
	AccountName    string // AWS account name in config file (interactive and non-interactive mode)
	RoleName       string // AWS Role name in config file
	RoleARN        string // AWS role ARN
	PrincipalARN   string // AWS Principal ARN
	Region         string // AWS Region
	Duration       int    // Credential expiration duration
	ConfigFilePath string // Path to a config file
	Interactive    bool   // Interactive mode flag
	OutputFile     string // Credentials output file path
	OutputFormat   string // Credential output format
	AwsCliProfile  string // AWS CLI profile name (for `profile` output format)
	Shell          bool   // Shell flag

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
	if err := app.Cli.Run(os.Args); err != nil {
		fmt.Println("Error: ", err)
		os.Exit(1)
	}
}

// Helpers
func writeBytes(w io.Writer, b []byte) error {
	_, err := w.Write(b)
	return err
}

func promptIfEmpty(interactive bool, cur *string, label, key, typ string, requiredErr error) error {
	if *cur != "" {
		return nil
	}
	if !interactive {
		return requiredErr
	}
	val, err := PromptSimple(label, key, typ)
	if err != nil {
		return err
	}
	*cur = val
	return nil
}

func validateIfSet(v string, validator func(string) error) error {
	if v == "" || validator == nil {
		return nil
	}
	return validator(v)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// CLI init
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
				_, _ = fmt.Fprintf(cCtx.App.Writer, "\n# **Warning:**\n# Config file %s not found\n\n", app.ConfigFilePath)
				return nil
			}
			app.Config = cfgFile

			if len(app.Config.Accounts) <= 0 {
				_, _ = fmt.Fprintf(cCtx.App.Writer, "\n# **Warning:**\n# Not found any accounts in the config file %s\n\n", app.ConfigFilePath)
				return nil
			}

			return nil
		},

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
				Name:        "role-name",
				Usage:       "AWS Role name (indicated in the config file)",
				EnvVars:     []string{"J2A_ROLE_NAME"},
				Destination: &app.RoleName,
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
				Destination: &app.AwsCliProfile,
			},
			&cli.BoolFlag{
				Name:        "shell",
				Aliases:     []string{"s"},
				Usage:       "Launch a shell with AWS credentials",
				EnvVars:     []string{"J2A_SHELL"},
				Value:       false,
				Destination: &app.Shell,
			},
		},
		Action: func(cCtx *cli.Context) error {
			if err := promptOptions(app); err != nil {
				_ = cli.ShowCommandHelp(cCtx, cCtx.Command.Name)
				return err
			}

			if app.Shell {
				return shell(cCtx, app)
			}
			return output(cCtx, app)
		},
	}
}

// Input prompting and validation
func promptOptions(app *App) error {
	// Resolve account
	account, err := resolveAccount(app)
	if err != nil {
		return err
	}
	if account != nil {
		fromAccountToAppConfig(*account, app)
	}

	// Role resolution: prefer RoleARN, else map RoleName → RoleARN (with account),
	// else interactive prompt (if account has roles), else require RoleARN.
	if app.RoleARN == "" {
		switch {
		case app.RoleName != "" && account != nil:
			arn, err := account.FindAWSRoleArnByName(app.RoleName)
			if err != nil {
				return err
			}
			app.RoleARN = arn.Arn
		case app.Interactive && account != nil && len(account.AWSRoleArns) > 0:
			app.RoleARN, err = PromptRoleArn(*account)
			if err != nil {
				return err
			}
		case app.Interactive:
			app.RoleARN, err = PromptSimple("Role ARN", "role-arn", "simple")
			if err != nil {
				return err
			}
		default:
			return errors.New(AwsRoleArnIsRequired)
		}
	} else if err := validateIfSet(app.RoleARN, validators["role-arn"]); err != nil {
		return err
	}

	// Region: prefer provided; else prompt from account regions; else general list.
	if app.Region == "" {
		if app.Interactive {
			if account != nil && len(account.AWSRegions) > 0 {
				app.Region, err = PromptRegion(account.AWSRegions)
			} else {
				app.Region, err = PromptRegion(aws.RegionsList)
			}
			if err != nil {
				return err
			}
		} else {
			return errors.New(AwsRegionIsRequired)
		}
	} else if err := validateIfSet(app.Region, validators["region"]); err != nil {
		return err
	}

	// Email
	if err := promptIfEmpty(app.Interactive, &app.Email, "Email", "email", "simple", errors.New(EmailIsRequired)); err != nil {
		return err
	}
	if err := validateIfSet(app.Email, validators["email"]); err != nil {
		return err
	}

	// Password
	if err := promptIfEmpty(app.Interactive, &app.Password, "Password", "password", "masked", errors.New(PasswordIsRequired)); err != nil {
		return err
	}
	if err := validateIfSet(app.Password, validators["password"]); err != nil {
		return err
	}

	// IDP URL
	if err := promptIfEmpty(app.Interactive, &app.IdpURL, "IDP URL", "idp-url", "simple", errors.New(IdpURLRequred)); err != nil {
		return err
	}
	if err := validateIfSet(app.IdpURL, validators["idp-url"]); err != nil {
		return err
	}

	// Principal ARN
	if err := promptIfEmpty(app.Interactive, &app.PrincipalARN, "Principal ARN", "principal-arn", "simple", errors.New(AwsPrincipalUrlIsRequired)); err != nil {
		return err
	}
	if err := validateIfSet(app.PrincipalARN, validators["principal-arn"]); err != nil {
		return err
	}

	// Output format (validate if provided; keep default if empty)
	if app.OutputFormat != "" {
		if err := validateIfSet(app.OutputFormat, validators["output-format"]); err != nil {
			return err
		}
	}

	// AWS CLI profile name required for cli / cli-stdout
	if app.OutputFormat == "cli" || app.OutputFormat == "cli-stdout" {
		if app.AwsCliProfile == "" {
			if app.Interactive {
				val, err := PromptSimple("AWS Cli profile name", "skip", "simple")
				if err != nil {
					return err
				}
				app.AwsCliProfile = val
			} else {
				return errors.New(AwsCliProfileNameIsRequired)
			}
		}
	}

	// MFA (optional); if set, validate format
	if app.MfaToken == "" && app.Interactive {
		val, err := PromptSimple("MFA Token Or MFA Secret", "skip", "simple")
		if err != nil {
			return err
		}
		app.MfaToken = val
	}
	if err := validateIfSet(app.MfaToken, validators["mfa"]); err != nil {
		return err
	}

	return nil
}

// Resolve account by interactive selection or by name; returns nil if not chosen/available.
func resolveAccount(app *App) (*config.Account, error) {
	if len(app.Config.Accounts) == 0 {
		if app.AccountName != "" {
			return nil, errors.New(AccountNameCantBeUsed)
		}
		return nil, nil
	}

	// Interactive: no name → prompt; with name → find
	if app.Interactive {
		if app.AccountName == "" {
			acc, err := PromptAccount(app.Config.GetAccounts())
			if err != nil {
				return nil, err
			}
			return &acc, nil
		}
		acc, err := app.Config.FindAccountByName(app.AccountName)
		if err != nil {
			return nil, err
		}
		return &acc, nil
	}

	// Non-interactive: only if name provided
	if app.AccountName == "" {
		return nil, nil
	}
	acc, err := app.Config.FindAccountByName(app.AccountName)
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// Get credentials flow
func getCredentials(email, password, idpURL, mfa, principalARN, roleARN, region string, duration int) (cred aws.AwsSamlOutput, err error) {
	// If MFA value length > 6, treat as secret and derive TOTP
	if len(mfa) > 6 {
		mfa, err = totp.GetToken(mfa)
		if err != nil {
			return cred, err
		}
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
	return cred, nil
}

// Copy config.Account into App, preferring explicit CLI overrides already set in App.
func fromAccountToAppConfig(account config.Account, app *App) {
	app.AccountName = firstNonEmpty(app.AccountName, account.Name)
	app.Email = firstNonEmpty(app.Email, account.Email)
	app.Password = firstNonEmpty(app.Password, account.Password)
	app.IdpURL = firstNonEmpty(app.IdpURL, account.IdpURL)
	app.MfaToken = firstNonEmpty(app.MfaToken, account.MFASecret)
	app.PrincipalARN = firstNonEmpty(app.PrincipalARN, account.AWSPrincipalArn)

	// Duration: use app if non-zero, else account if non-zero
	if app.Duration == 0 && account.Duration != 0 {
		app.Duration = account.Duration
	}

	// AWS CLI profile: prefer explicit flag; else account value; else account name
	switch {
	case app.AwsCliProfile != "":
		// keep
	case account.AwsCliProfile != "":
		app.AwsCliProfile = account.AwsCliProfile
	default:
		app.AwsCliProfile = account.Name
	}
}

// Output credentials in selected format
func output(ctx *cli.Context, app *App) error {
	cred, err := getCredentials(app.Email, app.Password, app.IdpURL, app.MfaToken, app.PrincipalARN, app.RoleARN, app.Region, app.Duration)
	if err != nil {
		return err
	}

	switch app.OutputFormat {
	case "cli": // store as aws-cli credentials (~/.aws/credentials)
		filePath := filepath.Join(UserHomeDir(), ".aws", "credentials")
		c, _ := cred.ToProfile(app.AwsCliProfile, filePath)
		if err := os.WriteFile(filePath, c, 0600); err != nil {
			return err
		}
	case "env": // store as env file (~/.jc2aws.env)
		filePath := filepath.Join(UserHomeDir(), ".jc2aws.env")
		if err := os.WriteFile(filePath, []byte(cred.PrintEnv()), 0600); err != nil {
			return err
		}
	case "cli-stdout": // print aws-cli credentials to STDOUT
		c, _ := cred.ToProfile(app.AwsCliProfile, "")
		if err := writeBytes(ctx.App.Writer, c); err != nil {
			return err
		}
	case "env-stdout": // print env variables to STDOUT
		if _, err := io.WriteString(ctx.App.Writer, cred.PrintEnv()); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported output format: %s", app.OutputFormat)
	}

	return nil
}

// Start interactive shell with AWS credentials environment variables
func shell(ctx *cli.Context, app *App) error {
	cred, err := getCredentials(app.Email, app.Password, app.IdpURL, app.MfaToken, app.PrincipalARN, app.RoleARN, app.Region, app.Duration)
	if err != nil {
		return err
	}

	env := cred.ToEnv()    // []string with creds
	sysEnv := os.Environ() // current env
	curShell := os.Getenv("SHELL")
	if curShell == "" {
		// sensible fallback; avoids exec error on minimal environments
		curShell = "/bin/sh"
	}

	scriptName := ctx.Args().First()
	var cmd *exec.Cmd
	if scriptName != "" {
		cmd = exec.Command(curShell, "-i", scriptName)
	} else {
		cmd = exec.Command(curShell, "-i")
	}
	cmd.Env = append(sysEnv, env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
