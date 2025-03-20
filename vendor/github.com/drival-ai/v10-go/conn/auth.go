package conn

import (
	"context"
	"fmt"
	"os"
)

type hardcodedAuth struct{}

// We require the VORTEX_API_KEY environment variable here.
func (h hardcodedAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	key := os.Getenv("VORTEX_API_KEY")
	authType := "Bearer"
	ftoken := fmt.Sprintf("%s %s", authType, key)
	return map[string]string{"authorization": ftoken}, nil
}

func (hardcodedAuth) RequireTransportSecurity() bool { return false }

func NewRpcCredentials() hardcodedAuth { return hardcodedAuth{} }
