// package emulator contains logic for interacting with the Firestore emulator
package emulator

import (
	"context"
)

// This environment variable may contain the network address where the Firestore emulator is bound.
const AddressEnvVar = "FIRESTORE_EMULATOR_HOST"

type emulatorCreds struct{}

func (ec emulatorCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer owner"}, nil
}
func (ec emulatorCreds) RequireTransportSecurity() bool {
	return false
}

// An instance of grpc.PerRPCCredentials that will configure a client to act as
// an admin for the Firestore emulator. It always hardcodes the "authorization"
// metadata field to contain "Bearer owner", which the Firestore emulator
// accepts as valid admin credentials.
var AdminCreds = emulatorCreds{}
