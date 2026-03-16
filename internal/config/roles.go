package config

type AWSRole struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Arn         string `yaml:"arn"`
}
