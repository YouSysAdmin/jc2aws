// Package providers constructs cloud.Provider implementations by name.
//
// It is the only package that imports every vendor implementation, which keeps
// internal/cloud free of vendor SDKs and avoids an import cycle between the
// contract and the packages that satisfy it.
package providers

import (
	"fmt"
	"strings"

	"github.com/yousysadmin/jc2aws/internal/alibaba"
	"github.com/yousysadmin/jc2aws/internal/aws"
	"github.com/yousysadmin/jc2aws/internal/cloud"
)

// Get returns the provider registered under name. The empty string resolves to
// cloud.DefaultName. Unknown names return an error wrapping
// cloud.ErrUnknownProvider.
func Get(name string) (cloud.Provider, error) {
	switch cloud.Normalize(name) {
	case cloud.NameAWS:
		return aws.New(), nil
	case cloud.NameAlibaba:
		return alibaba.New(), nil
	}

	return nil, fmt.Errorf("%w: %q (supported: %s)",
		cloud.ErrUnknownProvider, name, strings.Join(cloud.Names(), ", "))
}
