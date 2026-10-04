package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"gemstore/internal/middleware"
	"gemstore/internal/models"
	"gemstore/internal/repository"
)

// ErrEmailTaken re-exports repository.ErrEmailTaken for the same reason
// ErrOrderNotPending does in approval.go: handlers only import service.
var ErrEmailTaken = repository.ErrEmailTaken

// ErrInvalidCredentials covers both "no such email" and "wrong
// password" with one message — see the comment in Login for why those
// two cases are deliberately indistinguishable to the caller.
var ErrInvalidCredentials = errors.New("service: invalid email or password")

// ErrInvalidRefreshToken covers every way a refresh attempt can fail —
// no such token, expired, or already revoked — with one message and one
// error value, for the same reason ErrInvalidCredentials does: an
// attacker probing which specific case occurred shouldn't learn
// anything from the response.
var ErrInvalidRefreshToken = errors.New("service: invalid or expired refresh token")

// bcryptCost is intentionally above bcrypt.DefaultCost (10). 12 costs
// roughly 4x the hashing time — still well under human-perceptible
// delay for a single login request, but meaningfully more expensive for
// an attacker running offline brute-force attempts against a leaked
// hash table.
const bcryptCost = 12

// dummyHash is compared against on a login attempt for an email that
// doesn't exist, so that path takes roughly the same time as "email
// exists, password wrong." Without this, an attacker could tell the two
// apart by response latency alone (a real bcrypt compare vs. an
// immediate DB miss) and enumerate registered emails without ever
// seeing a different error message.
var dummyHash = mustHash("not-a-real-password-used-only-for-timing")

func mustHash(pw string) []byte {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	if err != nil {
		// Only ever runs once, at package init, on a fixed literal input
		// — if this fails, bcrypt itself is broken and nothing else in
		// the service would work either.
		panic(err)
	}
	return h
}

type AuthService struct {
	repo            *repository.UserRepository
	refreshRepo     *repository.RefreshTokenRepository
	jwtSecret       []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewAuthService(
	repo *repository.UserRepository,
	refreshRepo *repository.RefreshTokenRepository,
	jwtSecret []byte,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
) *AuthService {
	return &AuthService{
		repo:            repo,
		refreshRepo:     refreshRepo,
		jwtSecret:       jwtSecret,
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
	}
}

// AuthResult is what SignUp, Login, and RefreshAccessToken all produce:
// the account plus a ready-to-use access/refresh token pair. The
// handler (auth_handler.go) puts both tokens into HttpOnly cookies and
// never echoes them into a JSON body — see models.AuthResponse's doc
// comment for why.
type AuthResult struct {
	User            models.User
	AccessToken     string
	AccessTokenTTL  time.Duration
	RefreshToken    string
	RefreshTokenTTL time.Duration
}

// SignUp registers a new customer account. Role is hardcoded to
// 'customer' here — never read from the request — so public signup can
// never mint an admin no matter what a client sends. See cmd/seedadmin
// for creating the store owner's admin account out of band.
func (s *AuthService) SignUp(
	ctx context.Context,
	email, password, fullName string,
	phone *string,
) (*AuthResult, error) {
	email = normalizeEmail(email)

	if _, err := mail.ParseAddress(email); err != nil {
		return nil, ErrInvalidInput{Field: "email", Reason: "is not a valid email address"}
	}
	if len(password) < 8 {
		return nil, ErrInvalidInput{Field: "password", Reason: "must be at least 8 characters"}
	}
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return nil, ErrInvalidInput{Field: "full_name", Reason: "is required"}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("service: hash password: %w", err)
	}

	created, err := s.repo.Create(ctx, models.User{
		Email:        email,
		PasswordHash: string(hash),
		FullName:     fullName,
		Phone:        phone,
		Role:         models.RoleCustomer,
	})
	if err != nil {
		if errors.Is(err, repository.ErrEmailTaken) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}

	return s.issueResult(ctx, *created)
}

// Login verifies email/password and issues a token pair on success.
//
// Deliberately returns the same ErrInvalidCredentials whether the email
// doesn't exist or the password is wrong, and burns equivalent time in
// the "doesn't exist" branch via a dummy bcrypt compare — see dummyHash.
// A login endpoint that returns "no such user" vs. "wrong password", or
// that responds faster for nonexistent emails, hands an attacker a free
// account-enumeration oracle.
func (s *AuthService) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	email = normalizeEmail(email)

	user, err := s.repo.GetByEmail(ctx, email)
	if errors.Is(err, repository.ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issueResult(ctx, *user)
}

// RefreshAccessToken exchanges a valid, unexpired, unrevoked refresh
// token for a new access token AND a new refresh token — the old
// refresh token is revoked in the same call, so it can never be reused.
// This rotation is what makes stolen-token detection possible (see
// below), and it also shrinks the window during which a role change
// (promotion, demotion, ban) since the original login goes unnoticed:
// every refresh re-fetches the user fresh from the DB rather than
// trusting anything cached, so a role change takes effect the next time
// the access token would otherwise have expired — at most
// accessTokenTTL later, not whenever the user happens to log in again.
//
// If the presented token has ALREADY been revoked — meaning it was
// already used once (normal rotation) or explicitly revoked (logout) —
// that's treated as a signal the token may have been stolen and
// replayed: every refresh token for that user is revoked immediately,
// forcing every session to re-authenticate. A correctly-behaving client
// only ever presents a given refresh token once, by construction (each
// refresh response sets a brand new cookie); a second presentation is
// not something normal operation produces on its own.
func (s *AuthService) RefreshAccessToken(ctx context.Context, rawToken string) (*AuthResult, error) {
	hash := hashToken(rawToken)

	stored, err := s.refreshRepo.GetByHash(ctx, hash)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, err
	}

	if stored.RevokedAt != nil {
		// This IS the security event worth knowing about, independent of
		// whether the cleanup below succeeds — log it unconditionally,
		// not just on a failure to revoke.
		slog.WarnContext(ctx, "auth: refresh token reuse detected — revoking all sessions for user",
			"user_id", stored.UserID)
		if revokeErr := s.refreshRepo.RevokeAllForUser(ctx, stored.UserID); revokeErr != nil {
			slog.ErrorContext(ctx, "auth: failed to revoke all refresh tokens after reuse detected",
				"user_id", stored.UserID, "error", revokeErr)
		}
		return nil, ErrInvalidRefreshToken
	}

	if time.Now().After(stored.ExpiresAt) {
		return nil, ErrInvalidRefreshToken
	}

	user, err := s.repo.GetByID(ctx, stored.UserID)
	if err != nil {
		return nil, err
	}

	if err := s.refreshRepo.Revoke(ctx, hash); err != nil {
		return nil, err
	}

	return s.issueResult(ctx, *user)
}

// Logout revokes rawToken so it can never be used again, then the
// handler clears both cookies regardless of whether this succeeds — see
// AuthHandler.Logout. rawToken == "" (no refresh_token cookie present)
// is treated as an already-logged-out no-op, not an error.
func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	if err := s.refreshRepo.Revoke(ctx, hashToken(rawToken)); err != nil {
		return err
	}
	return nil
}

// GetProfile backs GET /api/v1/auth/me.
func (s *AuthService) GetProfile(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	return s.repo.GetByID(ctx, userID)
}

func (s *AuthService) issueResult(ctx context.Context, user models.User) (*AuthResult, error) {
	accessToken, err := middleware.GenerateToken(s.jwtSecret, user.ID, user.Role, s.accessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("service: issue access token: %w", err)
	}

	rawRefresh, hashRefresh, err := generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("service: generate refresh token: %w", err)
	}
	if err := s.refreshRepo.Create(ctx, user.ID, hashRefresh, time.Now().Add(s.refreshTokenTTL)); err != nil {
		return nil, fmt.Errorf("service: store refresh token: %w", err)
	}

	return &AuthResult{
		User:            user,
		AccessToken:     accessToken,
		AccessTokenTTL:  s.accessTokenTTL,
		RefreshToken:    rawRefresh,
		RefreshTokenTTL: s.refreshTokenTTL,
	}, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// generateRefreshToken produces a cryptographically random 32-byte
// token (256 bits — well beyond brute-force range) and its hash in one
// call, since every caller needs both: the raw value to hand to the
// client, the hash to store.
func generateRefreshToken() (raw string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("service: read random bytes: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashToken(raw), nil
}

// hashToken is SHA-256, not bcrypt — unlike a password, a refresh token
// is already 256 bits of high-entropy random data, not a
// human-memorable secret vulnerable to dictionary/rainbow-table attacks,
// so there's nothing for bcrypt's deliberate slowness to defend against
// here. A fast cryptographic hash is the right tool; the token's own
// entropy is what makes it unguessable.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
