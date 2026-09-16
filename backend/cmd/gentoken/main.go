// cmd/gentoken is a development helper, not a production auth flow. It
// mints a signed JWT for a given user ID + role so you can exercise the
// authenticated routes (POST /orders/custom, /admin/orders/...) before a
// real login endpoint (password hashing + a users repository) exists.
//
// Usage:
//
//	export JWT_SECRET=...            # same value the API server uses
//	go run ./cmd/gentoken -user <uuid> -role admin
//	go run ./cmd/gentoken -user <uuid> -role customer -ttl 2h
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"gemstore/internal/middleware"
	"gemstore/internal/models"
)

func main() {
	userFlag := flag.String("user", "", "user UUID to embed in the token (required)")
	roleFlag := flag.String("role", "customer", "role to embed: customer or admin")
	ttlFlag := flag.Duration("ttl", 24*time.Hour, "token lifetime, e.g. 2h, 24h")
	flag.Parse()

	if *userFlag == "" {
		fmt.Fprintln(os.Stderr, "error: -user is required")
		flag.Usage()
		os.Exit(1)
	}

	userID, err := uuid.Parse(*userFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: -user is not a valid UUID: %v\n", err)
		os.Exit(1)
	}

	role := models.UserRole(*roleFlag)
	if role != models.RoleCustomer && role != models.RoleAdmin {
		fmt.Fprintf(os.Stderr, "error: -role must be %q or %q\n", models.RoleCustomer, models.RoleAdmin)
		os.Exit(1)
	}

	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		fmt.Fprintln(os.Stderr, "error: JWT_SECRET env var must be set (same value the API server uses)")
		os.Exit(1)
	}

	token, err := middleware.GenerateToken([]byte(secret), userID, role, *ttlFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(token)
}
