package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrUserNotFound              = errors.New("user not found")
	ErrSessionNotFound           = errors.New("session not found")
	ErrUsernameTaken             = errors.New("username already exists")
	ErrSetupAlreadyComplete      = errors.New("setup already complete")
	ErrBootstrapOwnerKeyConflict = errors.New("multiple document owners found")
)

type User struct {
	ID                int64
	Username          string
	PasswordHash      string
	OwnerKey          string
	IsAdmin           bool
	AccountStatus     string
	CheckoutExpiresAt *time.Time
	CreatedAt         time.Time
}

const (
	AccountStatusActive          = "active"
	AccountStatusCheckoutPending = "checkout_pending"
)

type Session struct {
	ID         int64
	UserID     int64
	TokenHash  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastSeenAt time.Time
}

type SessionUser struct {
	Session Session
	User    User
}

func (s *Store) CountUsers(ctx context.Context) (int64, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	var count int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, s.wrapDBError("count_users", started, fmt.Errorf("count users: %w", err))
	}

	s.logDBOperation("count_users", started, nil)
	return count, nil
}

func (s *Store) UsersExist(ctx context.Context) (bool, error) {
	count, err := s.CountUsers(ctx)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (s *Store) BootstrapOwnerKey(ctx context.Context) (string, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT owner_key FROM documents LIMIT 2`)
	if err != nil {
		return "", s.wrapDBError("bootstrap_owner_key", started, fmt.Errorf("query owner keys: %w", err))
	}
	defer rows.Close()

	var ownerKeys []string
	for rows.Next() {
		var ownerKey string
		if scanErr := rows.Scan(&ownerKey); scanErr != nil {
			return "", s.wrapDBError("bootstrap_owner_key", started, fmt.Errorf("scan owner key: %w", scanErr))
		}
		ownerKeys = append(ownerKeys, ownerKey)
	}
	if err := rows.Err(); err != nil {
		return "", s.wrapDBError("bootstrap_owner_key", started, fmt.Errorf("iterate owner keys: %w", err))
	}

	s.logDBOperation("bootstrap_owner_key", started, nil)
	switch len(ownerKeys) {
	case 0:
		return "", nil
	case 1:
		return ownerKeys[0], nil
	default:
		return "", ErrBootstrapOwnerKeyConflict
	}
}

func (s *Store) CreateInitialUser(ctx context.Context, username, passwordHash, ownerKey string) (User, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, s.wrapDBError("create_initial_user_begin", started, fmt.Errorf("begin transaction: %w", err))
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var count int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return User{}, s.wrapDBError("create_initial_user_count", started, fmt.Errorf("count users: %w", err))
	}
	if count > 0 {
		return User{}, ErrSetupAlreadyComplete
	}

	createdAt := time.Now().UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO users (username, password_hash, owner_key, is_admin, account_status, checkout_expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		username,
		passwordHash,
		ownerKey,
		1,
		AccountStatusActive,
		nil,
		createdAt,
	)
	if err != nil {
		if isUniqueConstraintError(err, "users.username") {
			return User{}, ErrUsernameTaken
		}
		return User{}, s.wrapDBError("create_initial_user_insert", started, fmt.Errorf("insert user: %w", err))
	}

	id, err := result.LastInsertId()
	if err != nil {
		return User{}, s.wrapDBError("create_initial_user_last_id", started, fmt.Errorf("read inserted user id: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return User{}, s.wrapDBError("create_initial_user_commit", started, fmt.Errorf("commit user insert: %w", err))
	}
	committed = true
	s.logDBOperation("create_initial_user", started, nil)

	return User{
		ID:            id,
		Username:      username,
		PasswordHash:  passwordHash,
		OwnerKey:      ownerKey,
		IsAdmin:       true,
		AccountStatus: AccountStatusActive,
		CreatedAt:     mustParseTimestamp(createdAt),
	}, nil
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash, ownerKey string) (User, error) {
	return s.createUser(ctx, username, passwordHash, ownerKey, false, AccountStatusActive, nil)
}

func (s *Store) CreateProvisionalUser(ctx context.Context, username, passwordHash, ownerKey string, checkoutExpiresAt time.Time) (User, error) {
	expiresAt := checkoutExpiresAt.UTC()
	return s.createUser(ctx, username, passwordHash, ownerKey, false, AccountStatusCheckoutPending, &expiresAt)
}

func (s *Store) createUser(ctx context.Context, username, passwordHash, ownerKey string, isAdmin bool, accountStatus string, checkoutExpiresAt *time.Time) (User, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	createdAt := time.Now().UTC().Format(time.RFC3339)
	isAdminValue := 0
	if isAdmin {
		isAdminValue = 1
	}
	var checkoutExpiresValue any
	if checkoutExpiresAt != nil {
		checkoutExpiresValue = checkoutExpiresAt.UTC().Format(time.RFC3339)
	}

	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO users (username, password_hash, owner_key, is_admin, account_status, checkout_expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		username,
		passwordHash,
		ownerKey,
		isAdminValue,
		accountStatus,
		checkoutExpiresValue,
		createdAt,
	)
	if err != nil {
		if isUniqueConstraintError(err, "users.username") {
			return User{}, ErrUsernameTaken
		}
		return User{}, s.wrapDBError("create_user", started, fmt.Errorf("insert user: %w", err))
	}

	id, err := result.LastInsertId()
	if err != nil {
		return User{}, s.wrapDBError("create_user_last_id", started, fmt.Errorf("read inserted user id: %w", err))
	}

	s.logDBOperation("create_user", started, nil)
	return User{
		ID:                id,
		Username:          username,
		PasswordHash:      passwordHash,
		OwnerKey:          ownerKey,
		IsAdmin:           isAdmin,
		AccountStatus:     accountStatus,
		CheckoutExpiresAt: cloneTimePointer(checkoutExpiresAt),
		CreatedAt:         mustParseTimestamp(createdAt),
	}, nil
}

func (s *Store) ActivateUser(ctx context.Context, userID int64) error {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	if _, err := s.db.ExecContext(
		ctx,
		`UPDATE users SET account_status = ?, checkout_expires_at = NULL WHERE id = ?`,
		AccountStatusActive,
		userID,
	); err != nil {
		return s.wrapDBError("activate_user", started, fmt.Errorf("activate user: %w", err))
	}

	s.logDBOperation("activate_user", started, nil)
	return nil
}

func (s *Store) DeleteExpiredProvisionalUsers(ctx context.Context, now time.Time) (int64, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id FROM users WHERE account_status = ? AND checkout_expires_at IS NOT NULL AND checkout_expires_at <= ?`,
		AccountStatusCheckoutPending,
		now.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, s.wrapDBError("delete_expired_provisional_query", started, fmt.Errorf("query expired provisional users: %w", err))
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, s.wrapDBError("delete_expired_provisional_scan", started, fmt.Errorf("scan expired provisional user: %w", err))
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, s.wrapDBError("delete_expired_provisional_iterate", started, fmt.Errorf("iterate expired provisional users: %w", err))
	}

	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
			return 0, s.wrapDBError("delete_expired_provisional_sessions", started, fmt.Errorf("delete sessions: %w", err))
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM billing_customers WHERE user_id = ?`, id); err != nil {
			return 0, s.wrapDBError("delete_expired_provisional_customers", started, fmt.Errorf("delete billing customers: %w", err))
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM billing_subscriptions WHERE user_id = ?`, id); err != nil {
			return 0, s.wrapDBError("delete_expired_provisional_subscriptions", started, fmt.Errorf("delete billing subscriptions: %w", err))
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM account_entitlements WHERE user_id = ?`, id); err != nil {
			return 0, s.wrapDBError("delete_expired_provisional_entitlements", started, fmt.Errorf("delete account entitlements: %w", err))
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ? AND account_status = ?`, id, AccountStatusCheckoutPending); err != nil {
			return 0, s.wrapDBError("delete_expired_provisional_users", started, fmt.Errorf("delete users: %w", err))
		}
	}

	s.logDBOperation("delete_expired_provisional_users", started, nil)
	return int64(len(ids)), nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, owner_key, is_admin, account_status, checkout_expires_at, created_at
		FROM users
		WHERE username = ? COLLATE NOCASE`,
		username,
	)

	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return User{}, err
		}
		return User{}, s.wrapDBError("read_user_by_username", started, err)
	}

	s.logDBOperation("read_user_by_username", started, nil)
	return user, nil
}

func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (Session, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	createdAt := time.Now().UTC()
	if err := s.deleteExpiredSessions(ctx, createdAt); err != nil {
		return Session{}, err
	}

	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO sessions (user_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)`,
		userID,
		tokenHash,
		expiresAt.UTC().Format(time.RFC3339),
		createdAt.Format(time.RFC3339),
		createdAt.Format(time.RFC3339),
	)
	if err != nil {
		return Session{}, s.wrapDBError("create_session", started, fmt.Errorf("insert session: %w", err))
	}

	id, err := result.LastInsertId()
	if err != nil {
		return Session{}, s.wrapDBError("create_session_last_id", started, fmt.Errorf("read inserted session id: %w", err))
	}

	s.logDBOperation("create_session", started, nil)
	return Session{
		ID:         id,
		UserID:     userID,
		TokenHash:  tokenHash,
		ExpiresAt:  expiresAt.UTC(),
		CreatedAt:  createdAt,
		LastSeenAt: createdAt,
	}, nil
}

func (s *Store) GetSessionUserByTokenHash(ctx context.Context, tokenHash string) (SessionUser, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	row := s.db.QueryRowContext(
		ctx,
		`SELECT
			s.id,
			s.user_id,
			s.token_hash,
			s.expires_at,
			s.created_at,
			s.last_seen_at,
			u.id,
			u.username,
			u.password_hash,
			u.owner_key,
			u.is_admin,
			u.account_status,
			u.checkout_expires_at,
			u.created_at
		FROM sessions s
		INNER JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ?`,
		tokenHash,
	)

	sessionUser, err := scanSessionUser(row)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return SessionUser{}, err
		}
		return SessionUser{}, s.wrapDBError("read_session_by_token_hash", started, err)
	}

	s.logDBOperation("read_session_by_token_hash", started, nil)
	return sessionUser, nil
}

func (s *Store) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return s.wrapDBError("delete_session_by_token_hash", started, fmt.Errorf("delete session: %w", err))
	}

	s.logDBOperation("delete_session_by_token_hash", started, nil)
	return nil
}

func (s *Store) TouchSession(ctx context.Context, sessionID int64, lastSeenAt time.Time) error {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	if _, err := s.db.ExecContext(
		ctx,
		`UPDATE sessions SET last_seen_at = ? WHERE id = ?`,
		lastSeenAt.UTC().Format(time.RFC3339),
		sessionID,
	); err != nil {
		return s.wrapDBError("touch_session", started, fmt.Errorf("touch session: %w", err))
	}

	s.logDBOperation("touch_session", started, nil)
	return nil
}

func (s *Store) deleteExpiredSessions(ctx context.Context, now time.Time) error {
	started := time.Now()
	if _, err := s.db.ExecContext(
		ctx,
		`DELETE FROM sessions WHERE expires_at <= ?`,
		now.UTC().Format(time.RFC3339),
	); err != nil {
		return s.wrapDBError("delete_expired_sessions", started, fmt.Errorf("delete expired sessions: %w", err))
	}

	s.logDBOperation("delete_expired_sessions", started, nil)
	return nil
}

func scanUser(row interface {
	Scan(dest ...any) error
}) (User, error) {
	var user User
	var isAdmin int64
	var accountStatus sql.NullString
	var checkoutExpiresAt sql.NullString
	var createdAt string
	if err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.OwnerKey, &isAdmin, &accountStatus, &checkoutExpiresAt, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("scan user: %w", err)
	}

	user.IsAdmin = isAdmin == 1
	user.AccountStatus = AccountStatusActive
	if accountStatus.Valid && strings.TrimSpace(accountStatus.String) != "" {
		user.AccountStatus = strings.TrimSpace(accountStatus.String)
	}
	if checkoutExpiresAt.Valid && strings.TrimSpace(checkoutExpiresAt.String) != "" {
		parsed := mustParseTimestamp(checkoutExpiresAt.String)
		user.CheckoutExpiresAt = &parsed
	}
	user.CreatedAt = mustParseTimestamp(createdAt)
	return user, nil
}

func scanSessionUser(row interface {
	Scan(dest ...any) error
}) (SessionUser, error) {
	var sessionUser SessionUser
	var expiresAt string
	var createdAt string
	var lastSeenAt string
	var isAdmin int64
	var accountStatus sql.NullString
	var checkoutExpiresAt sql.NullString
	var userCreatedAt string
	if err := row.Scan(
		&sessionUser.Session.ID,
		&sessionUser.Session.UserID,
		&sessionUser.Session.TokenHash,
		&expiresAt,
		&createdAt,
		&lastSeenAt,
		&sessionUser.User.ID,
		&sessionUser.User.Username,
		&sessionUser.User.PasswordHash,
		&sessionUser.User.OwnerKey,
		&isAdmin,
		&accountStatus,
		&checkoutExpiresAt,
		&userCreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionUser{}, ErrSessionNotFound
		}
		return SessionUser{}, fmt.Errorf("scan session user: %w", err)
	}

	sessionUser.User.IsAdmin = isAdmin == 1
	sessionUser.User.AccountStatus = AccountStatusActive
	if accountStatus.Valid && strings.TrimSpace(accountStatus.String) != "" {
		sessionUser.User.AccountStatus = strings.TrimSpace(accountStatus.String)
	}
	if checkoutExpiresAt.Valid && strings.TrimSpace(checkoutExpiresAt.String) != "" {
		parsed := mustParseTimestamp(checkoutExpiresAt.String)
		sessionUser.User.CheckoutExpiresAt = &parsed
	}
	sessionUser.Session.ExpiresAt = mustParseTimestamp(expiresAt)
	sessionUser.Session.CreatedAt = mustParseTimestamp(createdAt)
	sessionUser.Session.LastSeenAt = mustParseTimestamp(lastSeenAt)
	sessionUser.User.CreatedAt = mustParseTimestamp(userCreatedAt)
	return sessionUser, nil
}

func isUniqueConstraintError(err error, column string) bool {
	if err == nil {
		return false
	}

	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") &&
		strings.Contains(strings.ToLower(err.Error()), strings.ToLower(column))
}
