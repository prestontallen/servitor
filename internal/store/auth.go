package store

// Credential storage and verification for built-in users, GUI sessions and
// per-actor API tokens. These tables are NOT the ledger: they hold the
// credentials that authorize writes to it, and rows here are mutable
// (deleted on logout / token revoke) where the ledger is append-only.
//
// Hashing: passwords are argon2id, stored as PHC strings. Session values
// and API tokens are random 256-bit values shown once to the caller and
// stored only as sha256 hex — a dump of these tables yields nothing usable.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/argon2"
)

const (
	argonMemory  uint32 = 64 * 1024
	argonTime    uint32 = 1
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

// HashPassword derives an argon2id PHC string for storage.
func HashPassword(password string) string {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	key := argon2.IDKey([]byte(password), salt, argonMemory, argonTime, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key))
}

// VerifyPassword checks a password against a stored PHC string in constant
// time. A malformed stored hash never matches.
func VerifyPassword(stored, password string) bool {
	parts := strings.Split(stored, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var salt, want []byte
	var err error
	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return false
	}
	if want, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, argonMemory, argonTime, argonThreads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// randomToken returns 256 bits of hex entropy. Shown once; only the hash
// is stored.
func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// ErrUsersExist means first-run bootstrap was attempted on a non-empty
// users table.
var ErrUsersExist = errors.New("users already exist")

// CreateFirstUser inserts the first user, atomically only-if-empty — two
// racing bootstrap requests cannot both win. The actor defaults to
// human:<username> when empty.
func (s *Store) CreateFirstUser(ctx context.Context, username, password, actor string) error {
	if username == "" || password == "" {
		return fmt.Errorf("username and password must not be empty")
	}
	if actor == "" {
		actor = "human:" + username
	}
	tag, err := s.Pool.Exec(ctx,
		`INSERT INTO users (username, actor, pass_hash)
		 SELECT $1, $2, $3 WHERE NOT EXISTS (SELECT 1 FROM users)`,
		username, actor, HashPassword(password))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUsersExist
	}
	return nil
}

// UsersExist reports whether any user is provisioned. The GUI reads this
// (unauthenticated) to choose the first-run setup view over the login view.
func (s *Store) UsersExist(ctx context.Context) (bool, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n > 0, err
}

// CreateUser inserts a user with an argon2id-hashed password. The actor is
// bound at creation and never changes.
func (s *Store) CreateUser(ctx context.Context, username, password, actor string) error {
	if username == "" || password == "" || actor == "" {
		return fmt.Errorf("username, password and actor must not be empty")
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO users (username, actor, pass_hash) VALUES ($1, $2, $3)`,
		username, actor, HashPassword(password))
	if err != nil && strings.Contains(err.Error(), "users_username_ci_uidx") {
		return fmt.Errorf("username claimed: %s", username)
	}
	return err
}

// VerifyLogin checks credentials and returns the bound actor. Unknown user
// and wrong password are the same error — the response must not say which.
func (s *Store) VerifyLogin(ctx context.Context, username, password string) (string, error) {
	var stored, actor string
	err := s.Pool.QueryRow(ctx,
		`SELECT pass_hash, actor FROM users WHERE lower(username)=lower($1)`, username).
		Scan(&stored, &actor)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("invalid credentials")
		}
		return "", err
	}
	if !VerifyPassword(stored, password) {
		return "", fmt.Errorf("invalid credentials")
	}
	return actor, nil
}

// CreateSession mints a session for a user and returns the raw cookie
// value. Sessions live in the database so a daemon restart does not log
// the GUI out.
func (s *Store) CreateSession(ctx context.Context, userID int64) (string, error) {
	raw := randomToken()
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id) VALUES ($1, $2)`,
		hashToken(raw), userID)
	return raw, err
}

// SessionActor resolves a session cookie value to its user's actor.
func (s *Store) SessionActor(ctx context.Context, raw string) (string, bool) {
	var actor string
	err := s.Pool.QueryRow(ctx,
		`SELECT u.actor FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = $1`,
		hashToken(raw)).Scan(&actor)
	return actor, err == nil
}

// SessionUser resolves a session cookie to its user's username and actor.
func (s *Store) SessionUser(ctx context.Context, raw string) (string, string, bool) {
	var username, actor string
	err := s.Pool.QueryRow(ctx,
		`SELECT u.username, u.actor FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = $1`,
		hashToken(raw)).Scan(&username, &actor)
	return username, actor, err == nil
}

// SessionUserID resolves a session cookie to its user row id.
func (s *Store) SessionUserID(ctx context.Context, raw string) (int64, bool) {
	var id int64
	err := s.Pool.QueryRow(ctx,
		`SELECT user_id FROM sessions WHERE token_hash = $1`, hashToken(raw)).Scan(&id)
	return id, err == nil
}

// ChangePassword verifies the current password, rehashes the new one, and
// revokes every other session of the user — the session that changed the
// password survives.
func (s *Store) ChangePassword(ctx context.Context, userID int64, current, keepSessionRaw, next string) error {
	if next == "" {
		return fmt.Errorf("new password must not be empty")
	}
	var stored, keepHash string
	err := s.Pool.QueryRow(ctx,
		`SELECT u.pass_hash, COALESCE(s.token_hash, '')
		 FROM users u
		 LEFT JOIN sessions s ON s.user_id = u.id AND s.token_hash = $2
		 WHERE u.id = $1`, userID, hashToken(keepSessionRaw)).Scan(&stored, &keepHash)
	if err != nil {
		return fmt.Errorf("invalid credentials")
	}
	if !VerifyPassword(stored, current) {
		return fmt.Errorf("invalid credentials")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE users SET pass_hash = $1 WHERE id = $2`, HashPassword(next), userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`,
		userID, keepHash); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteSession revokes one session (logout).
func (s *Store) DeleteSession(ctx context.Context, raw string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(raw))
	return err
}

// CreateAPIToken mints a per-actor token and returns the raw value, shown
// once. The actor is anything the ledger models: agent:<name>, human:<name>.
func (s *Store) CreateAPIToken(ctx context.Context, actor, label string) (string, error) {
	if actor == "" {
		return "", fmt.Errorf("actor must not be empty")
	}
	raw := randomToken()
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO api_tokens (token_hash, actor, label) VALUES ($1, $2, $3)`,
		hashToken(raw), actor, label)
	return raw, err
}

// TokenActor resolves a raw per-actor token to its bound actor.
func (s *Store) TokenActor(ctx context.Context, raw string) (string, bool) {
	var actor string
	err := s.Pool.QueryRow(ctx,
		`SELECT actor FROM api_tokens WHERE token_hash = $1`, hashToken(raw)).Scan(&actor)
	return actor, err == nil
}
