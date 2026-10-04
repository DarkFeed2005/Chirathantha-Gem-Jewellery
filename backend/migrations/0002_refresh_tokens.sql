-- =============================================================================
-- Migration 0002: refresh_tokens
-- Supports the httpOnly-cookie + short-lived-access-token +
-- revocable-refresh-token auth flow (see internal/service/auth_service.go).
-- =============================================================================

BEGIN;

CREATE TABLE refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- SHA-256 hash of the raw token, hex-encoded — never the raw token
    -- itself. Same rationale as password_hash on users: if this table
    -- ever leaks, the attacker gets hashes they can't turn back into
    -- usable session tokens, not live credentials for every active
    -- session.
    token_hash  TEXT NOT NULL,

    expires_at  TIMESTAMPTZ NOT NULL,

    -- NULL = still valid. Set on: normal rotation (a refresh token is
    -- used exactly once, then revoked and replaced — see
    -- AuthService.RefreshAccessToken), explicit logout, or a mass
    -- revoke-all-for-user triggered by reuse-of-an-already-revoked-token
    -- detection (a signal of possible theft).
    revoked_at  TIMESTAMPTZ,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT refresh_tokens_token_hash_unique UNIQUE (token_hash)
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id);

-- Speeds up a future periodic cleanup job (DELETE FROM refresh_tokens
-- WHERE expires_at < now() OR revoked_at IS NOT NULL) — not implemented
-- yet, so rows accumulate indefinitely until one exists; see the
-- backend README's assumptions list.
CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens (expires_at);

COMMIT;
