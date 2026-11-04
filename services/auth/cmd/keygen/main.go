// Command keygen prints a new Ed25519 signing key for auth-service.
// Usage: go run ./cmd/keygen   (then put the output in your secret manager / .env)
package main

import (
	"fmt"
	"os"

	"github.com/yourorg/wallet-service/services/auth/internal/security"
)

func main() {
	key, err := security.GenerateKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("JWT_PRIVATE_KEY=%s\n", key)
}
