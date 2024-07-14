package config

type AWSRole struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Arn         string `yaml:"arn"`
}

// GetName return role name
func (r *AWSRole) GetName() string {
	return r.Name
}

// GetArn return role arn
func (r *AWSRole) GetArn() string {
	return r.Arn
}
