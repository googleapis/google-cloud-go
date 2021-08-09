// Package detect is used find information from the environment.
package detect

import (
	"context"
	"errors"
	"fmt"
	"os"

	"google.golang.org/api/option"
	"google.golang.org/api/transport"
)

const projectIDSentinel = "*detect-project-id*"

// ProjectID tries to detect the project ID from the environment if the sentinel
// value, "*detect-project-id*", is sent. It looks in the following order:
//   1. GOOGLE_CLOUD_PROJECT envvar
//   2. ADC creds.ProjectID
//   3. A static value if the environment is emulated.
func ProjectID(ctx context.Context, projectID string, emulatorEnvVar string, opts ...option.ClientOption) (string, error) {
	if projectID != projectIDSentinel {
		return projectID, nil
	}
	// 1. Try a well known environment variable
	if id := os.Getenv("GOOGLE_CLOUD_PROJECT"); id != "" {
		return id, nil
	}
	// 2. Try ADC
	creds, err := transport.Creds(ctx, opts...)
	if err != nil {
		return "", fmt.Errorf("fetching creds: %v", err)
	}
	// 3. If ADC does not work, and the environment is emulated, return a const value.
	if creds.ProjectID == "" && emulatorEnvVar != "" && os.Getenv(emulatorEnvVar) != "" {
		return "emulated-project", nil
	}
	// 4. If 1-3 don't work, error out
	if creds.ProjectID == "" {
		return "", errors.New("unable to detect projectID, please refer to docs for DetectProjectID")
	}
	// Success from ADC
	return creds.ProjectID, nil
}
