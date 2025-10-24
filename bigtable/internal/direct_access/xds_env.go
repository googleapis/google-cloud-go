package internal

import (
	"fmt"
	"os"
)

// CheckXDSBootstrapEnv checks that no GRPC_XDS_BOOTSTRAP or GRPC_XDS_BOOTSTRAP_CONFIG
// environment variables are set, as these would interfere with DirectPath's TD setup.
func CheckXDSBootstrapEnv() error {
	const xdsBootStrapEnvVar = "GRPC_XDS_BOOTSTRAP"
	const xdsBootStrapConfigEnvVar = "GRPC_XDS_BOOTSTRAP_CONFIG"
	if os.Getenv(xdsBootStrapEnvVar) != "" || os.Getenv(xdsBootStrapConfigEnvVar) != "" {
		return fmt.Errorf("DirectPath can not be used with environment variables |%v| or |%v|", xdsBootStrapEnvVar, xdsBootStrapConfigEnvVar)
	}
	return nil
}
