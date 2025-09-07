package main

import (
	"errors"
	"net/mail"
	"net/url"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/yousysadmin/jc2aws/internal/aws"
)

// Input parameters validators list
var validators = map[string]func(input string) error{
	"skip": func(input string) error { return nil },
	"email": func(input string) error {
		_, err := mail.ParseAddress(input)
		if err != nil {
			return errors.New("invalid e-mail address")
		}
		return nil
	},
	"password": func(input string) error {
		if len(input) < 8 {
			return errors.New("password must be at least 8 characters")
		}
		return nil
	},
	"idp-url": func(input string) error {
		_, err := url.ParseRequestURI(input)
		if err != nil {
			return errors.New("invalid idp url")
		}
		return nil
	},
	"role-arn": func(input string) error {
		_, err := arn.Parse(input)
		if err != nil {
			return errors.New("invalid role arn")
		}
		return nil
	},
	"principal-arn": func(input string) error {
		_, err := arn.Parse(input)
		if err != nil {
			return errors.New("invalid principal arn")
		}
		return nil
	},
	"region": func(input string) error {
		idx := slices.IndexFunc(aws.RegionsList, func(c string) bool { return c == input })
		if idx == -1 {
			return errors.New("invalid region")
		}
		return nil
	},
	"mfa": func(input string) error {
		if len(input) < 6 {
			return errors.New("mfa must be a 6-digit totp code or mfa secret string.")
		}
		return nil
	},
	"output-format": func(input string) error {
		formats := []string{"cli", "env", "cli-stdout", "env-stdout"}
		idx := slices.IndexFunc(formats, func(c string) bool { return c == input })
		if idx == -1 {
			return errors.New("invalid output format")
		}
		return nil
	},
}
