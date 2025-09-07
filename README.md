# jc2aws
CLI tool for getting temporary AWS credentials via Jumpcloud SSO

[![Stand with Ukraine](https://raw.githubusercontent.com/vshymanskyy/StandWithUkraine/main/banner2-direct.svg)](https://github.com/vshymanskyy/StandWithUkraine/blob/main/docs/README.md)

## Features:
- Support fully automate auth including MFA Token generate.
- Support manual (default), interactive and mixed modes
- Output credentials as AWS CLI profile or Environment variables (to file or STDOUT)
  - AWS CLI file path - $HOME/.aws/credentials
  - Environment vars - $HOME/.jc2aws.env
  - Run interactive shell or execute script - with got credentials as environment variables
- Any parameters not included in a config file can be set over flags or interactive mode
- Can use a configuration file, flags, and environment variables for customization, individually or in combination.

## Install

```shell
go install github.com/yousysadmin/jc2aws/cmd/jc2aws@latest
```

```shell
# By default install to $HOME/.bin dir
curl -L https://raw.githubusercontent.com/yousysadmin/jc2aws/master/scripts/install.sh | bash
```

## Usage

**IMPORTANT:** Jumpcloud only allows you to log in with one TOTP code once, in fact you can't login more than once every 30 seconds (TOTP code expiration time)

```
NAME:
   jc2aws - Get AWS credentials

USAGE:
   Get temporarily AWS credentials via Jumpcloud (SAML)

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --config value, -c value         Path to a config file (default: "/Users/yousysadmin/.jc2aws.yaml") [$J2A_CONFIG]
   --interactive, -i                Turn on interactive mode (default: false) [$J2A_INTERACTIVE]
   --email value, -e value          Jumpcloud user email [$J2A_EMAIL]
   --password value, -p value       Jumpcloud user password [$J2A_PASSWORD]
   --mfa value, -m value            Jumpcloud user MFA token [$J2A_MFA]
   --idp-url value                  Jumpcloud IDP URL (ex: https://sso.jumpcloud.com/saml2/my-aws-prod) [$J2A_IDP_URL]
   --role-name value                AWS Role name (indicated in the config file) [$J2A_ROLE_NAME]
   --role-arn value                 AWS Role ARN (ex: arn:aws:iam::ACCOUNT-ID:role/admin) [$J2A_ROLE_ARN]
   --principal-arn value            AWS Identity provider ARN (ex: arn:aws:iam::ACCOUNT-ID:saml-provider/jumpcloud) [$J2A_PRINCIPAL_ARN]
   --region value, -r value         AWS region (ex: us-west-2) [$J2A_AWS_REGION]
   --duration value, -d value       AWS credential expiration time (default: 3600) [$J2A_DURATION]
   --account value, -a value        Account name present in a config [$J2A_ACCOUNT]
   --output-format value, -f value  Credential output format (ex: cli, env, cli-stdout, env-stdout) (default: "cli") [$J2A_OUTPUT_FORMAT]
   --aws-cli-profile-name value     AWS profile name used for store credentials [$J2A_AWS_CLI_PROFILE_NAME]
   --shell, -s                      Launch a shell with AWS credentials (default: false) [$J2A_SHELL]
   --help, -h                       show help
```
### Interactive
```shell
# Interactive mode 
jc2aws -i
Use the arrow keys to navigate: ↓ ↑ → ←  and / toggles search
Select account:
  > my-prod
    my-stage

--------- Account Properties ----------
Description:        Production account
Roles:              admin, read-only
Regions:            ca-central-1, us-east-1
E-mail              Present
Password            Present
MFA                 Present
Duration:           3600

# You can set flags --account, --role-name / --role-arn, --region for skip some interactive step
```

### Manual
```shell
# Full Manual mode 
jc2aws --email my-user@example.com \
       --password "my-password" \
       --idp-url "https://sso.jumpcloud.com/saml2/my-prod" \
       --role-arn "arn:aws:iam::0000000:role/jumpcloud-admin" \
       --principal-arn "arn:aws:iam::0000000:saml-provider/jumpcloud" \
       --region ca-central-1 \
       --mfa "123456" # or --mfa "YourMFASecret" for automate generate MFA token

# Manual from config file
jc2aws --account my-prod \
       --role-name=admin \      
       # or --role-arn for define custom role
       --region ca-central-1
```

### Running a shell or execute script
You can use flag `--shell` or `-s` for run shell with a got credentials or run local script.

_If you do not specify the script name as arg, the interactive shell will be launched, otherwise, the specified script will be launched._
```shell
jc2aws ... [-s | --shel] script.sh
```

## Config file
```yaml
# $HOME/.jc2aws.yaml
---
# default login for all accounts if an account is not set separately
default_email: "user@yousysadmin.com"

# default password for all accounts if an account is not set separately
default_password: "MyVeryCoolPassword"

# default MFA secret for all accounts if an account is not set separately
default_mfa_token_secret: "MyMFASecret"

# AWS accounts configs
accounts:
  # Name
  - name: my-prod
    # Description
    description: "Production account"
    # Jumpcloud user Email
    Email: "user@example.com"
    # Jumpcloud user Password
    Password: "MyVeryCoolPassword"
    # MFA Secret
    mfa_token_secret: "MyMFASecret"
    # Principal ARN
    aws_principal_arn: "arn:aws:iam::0000000000:saml-provider/jumpcloud"
    # Roles list
    aws_role_arns:
      # Name
      - name: admin
        # Description
        description: "AWS Role with full access"
        # ARN
        arn: "arn:aws:iam::0000000000:role/jumpcloud-admin"
      - name: read-only
        description: "AWS Role with read-only access"
        arn: "arn:aws:iam::0000000000:role/jumpcloud-readonly"
    # Regions list
    aws_regions:
      - "ca-central-1"
      - "us-east-1"
    # Jumpcloud IDP URL
    jc_idp_url: https://sso.jumpcloud.com/saml2/my-prod
    # Session Duration
    session_timeout: 3600

  - name: my-stage
    description: "Staging account"
    aws_principal_arn: "arn:aws:iam::0000000000:saml-provider/jumpcloud"
    aws_role_arns:
      - name: admin
        description: "AWS Role with full access"
        arn: "arn:aws:iam::0000000000:role/jumpcloud-admin"
      - name: read-only
        description: "AWS Role with read-only access"
        arn: "arn:aws:iam::0000000000:role/jumpcloud-readonly"
    aws_regions:
      - "ca-central-1"
      - "us-east-1"
    jc_idp_url: https://sso.jumpcloud.com/saml2/my-stage
    session_timeout: 3600

```
