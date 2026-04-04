package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

type userRow struct {
	ID           string
	Email        string
	FullName     string
	SystemRole   string
	PasswordHash string
	IsActive     bool
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type membershipRow struct {
	ID        string
	UserID    string
	ClubID    string
	ClubName  string
	ClubRole  string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) EnsureBootstrapSysAdmin(
	ctx context.Context,
	email string,
	fullName string,
	passwordHash string,
) error {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" || passwordHash == "" {
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	var existingID string
	err = tx.QueryRow(
		ctx,
		`SELECT id FROM users WHERE LOWER(email) = $1 AND deleted_at IS NULL LIMIT 1`,
		normalizedEmail,
	).Scan(&existingID)
	switch {
	case err == nil:
		_, err = tx.Exec(
			ctx,
			`UPDATE users
			 SET full_name = $2,
			     password_hash = $3,
			     system_role = 'sys_admin',
			     is_active = TRUE,
			     updated_at = $4
			 WHERE id = $1`,
			existingID,
			fullName,
			passwordHash,
			now,
		)
		if err != nil {
			return err
		}
	case errors.Is(err, pgx.ErrNoRows):
		_, err = tx.Exec(
			ctx,
			`INSERT INTO users (
				id, email, full_name, password_hash, system_role, is_active, created_at, updated_at
			) VALUES ($1, $2, $3, $4, 'sys_admin', TRUE, $5, $5)`,
			uuid.NewString(),
			normalizedEmail,
			fullName,
			passwordHash,
			now,
		)
		if err != nil {
			return err
		}
	default:
		return err
	}

	return tx.Commit(ctx)
}

func (s *Store) FindUserByEmail(ctx context.Context, email string) (*userRow, error) {
	row := userRow{}
	err := s.pool.QueryRow(
		ctx,
		`SELECT id, email, full_name, system_role, password_hash, is_active, last_login_at, created_at, updated_at
		 FROM users
		 WHERE LOWER(email) = LOWER($1) AND deleted_at IS NULL
		 LIMIT 1`,
		strings.TrimSpace(email),
	).Scan(
		&row.ID,
		&row.Email,
		&row.FullName,
		&row.SystemRole,
		&row.PasswordHash,
		&row.IsActive,
		&row.LastLoginAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &row, nil
}

func (s *Store) FindUserByID(ctx context.Context, userID string) (*userRow, error) {
	row := userRow{}
	err := s.pool.QueryRow(
		ctx,
		`SELECT id, email, full_name, system_role, password_hash, is_active, last_login_at, created_at, updated_at
		 FROM users
		 WHERE id = $1 AND deleted_at IS NULL
		 LIMIT 1`,
		userID,
	).Scan(
		&row.ID,
		&row.Email,
		&row.FullName,
		&row.SystemRole,
		&row.PasswordHash,
		&row.IsActive,
		&row.LastLoginAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &row, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]userRow, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, email, full_name, system_role, password_hash, is_active, last_login_at, created_at, updated_at
		 FROM users
		 WHERE deleted_at IS NULL
		 ORDER BY full_name ASC, email ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]userRow, 0)
	for rows.Next() {
		var row userRow
		if err := rows.Scan(
			&row.ID,
			&row.Email,
			&row.FullName,
			&row.SystemRole,
			&row.PasswordHash,
			&row.IsActive,
			&row.LastLoginAt,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}

	return result, rows.Err()
}

func (s *Store) CreateUser(
	ctx context.Context,
	email string,
	fullName string,
	passwordHash string,
	systemRole string,
	isActive bool,
) (*userRow, error) {
	now := time.Now().UTC()
	row := userRow{}
	err := s.pool.QueryRow(
		ctx,
		`INSERT INTO users (
			id, email, full_name, password_hash, system_role, is_active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		RETURNING id, email, full_name, system_role, password_hash, is_active, last_login_at, created_at, updated_at`,
		uuid.NewString(),
		strings.ToLower(strings.TrimSpace(email)),
		strings.TrimSpace(fullName),
		passwordHash,
		systemRole,
		isActive,
		now,
	).Scan(
		&row.ID,
		&row.Email,
		&row.FullName,
		&row.SystemRole,
		&row.PasswordHash,
		&row.IsActive,
		&row.LastLoginAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &row, nil
}

func (s *Store) UpdateUserActiveStatus(ctx context.Context, userID string, isActive bool) (*userRow, error) {
	row := userRow{}
	now := time.Now().UTC()
	err := s.pool.QueryRow(
		ctx,
		`UPDATE users
		 SET is_active = $2,
		     updated_at = $3
		 WHERE id = $1
		   AND deleted_at IS NULL
		 RETURNING id, email, full_name, system_role, password_hash, is_active, last_login_at, created_at, updated_at`,
		userID,
		isActive,
		now,
	).Scan(
		&row.ID,
		&row.Email,
		&row.FullName,
		&row.SystemRole,
		&row.PasswordHash,
		&row.IsActive,
		&row.LastLoginAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &row, nil
}

func (s *Store) UpdateUserPassword(ctx context.Context, userID string, passwordHash string) (*userRow, error) {
	row := userRow{}
	now := time.Now().UTC()
	err := s.pool.QueryRow(
		ctx,
		`UPDATE users
		 SET password_hash = $2,
		     updated_at = $3
		 WHERE id = $1
		   AND deleted_at IS NULL
		 RETURNING id, email, full_name, system_role, password_hash, is_active, last_login_at, created_at, updated_at`,
		userID,
		passwordHash,
		now,
	).Scan(
		&row.ID,
		&row.Email,
		&row.FullName,
		&row.SystemRole,
		&row.PasswordHash,
		&row.IsActive,
		&row.LastLoginAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &row, nil
}

func (s *Store) UpdateLastLoginAt(ctx context.Context, userID string, loggedAt time.Time) error {
	_, err := s.pool.Exec(
		ctx,
		`UPDATE users SET last_login_at = $2, updated_at = $2 WHERE id = $1`,
		userID,
		loggedAt,
	)
	return err
}

func (s *Store) ListMembershipsByUserID(ctx context.Context, userID string) ([]membershipRow, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT cm.id, cm.user_id, cm.club_id, c.name, cm.club_role, cm.is_active, cm.created_at, cm.updated_at
		 FROM club_memberships cm
		 INNER JOIN clubs c ON c.id = cm.club_id
		 WHERE cm.user_id = $1
		   AND cm.revoked_at IS NULL
		   AND c.deleted_at IS NULL
		 ORDER BY c.name ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]membershipRow, 0)
	for rows.Next() {
		var row membershipRow
		if err := rows.Scan(
			&row.ID,
			&row.UserID,
			&row.ClubID,
			&row.ClubName,
			&row.ClubRole,
			&row.IsActive,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}

	return result, rows.Err()
}

func (s *Store) FindMembershipByUserAndClub(
	ctx context.Context,
	userID string,
	clubID string,
) (*membershipRow, error) {
	row := membershipRow{}
	err := s.pool.QueryRow(
		ctx,
		`SELECT cm.id, cm.user_id, cm.club_id, c.name, cm.club_role, cm.is_active, cm.created_at, cm.updated_at
		 FROM club_memberships cm
		 INNER JOIN clubs c ON c.id = cm.club_id
		 WHERE cm.user_id = $1
		   AND cm.club_id = $2
		   AND cm.revoked_at IS NULL
		   AND c.deleted_at IS NULL
		 LIMIT 1`,
		userID,
		clubID,
	).Scan(
		&row.ID,
		&row.UserID,
		&row.ClubID,
		&row.ClubName,
		&row.ClubRole,
		&row.IsActive,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &row, nil
}

func (s *Store) CreateMembership(
	ctx context.Context,
	userID string,
	clubID string,
	clubRole string,
	isActive bool,
) (*membershipRow, error) {
	now := time.Now().UTC()
	row := membershipRow{}
	err := s.pool.QueryRow(
		ctx,
		`INSERT INTO club_memberships (
			id, user_id, club_id, club_role, is_active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $6)
		RETURNING id, user_id, club_id,
			(SELECT name FROM clubs WHERE id = $3),
			club_role, is_active, created_at, updated_at`,
		uuid.NewString(),
		userID,
		clubID,
		clubRole,
		isActive,
		now,
	).Scan(
		&row.ID,
		&row.UserID,
		&row.ClubID,
		&row.ClubName,
		&row.ClubRole,
		&row.IsActive,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &row, nil
}

func (s *Store) RevokeMembership(
	ctx context.Context,
	membershipID string,
) (*membershipRow, error) {
	row := membershipRow{}
	now := time.Now().UTC()
	err := s.pool.QueryRow(
		ctx,
		`UPDATE club_memberships cm
		 SET revoked_at = $2,
		     updated_at = $2
		 FROM clubs c
		 WHERE cm.id = $1
		   AND cm.club_id = c.id
		   AND cm.revoked_at IS NULL
		 RETURNING cm.id, cm.user_id, cm.club_id, c.name, cm.club_role, cm.is_active, cm.created_at, cm.updated_at`,
		membershipID,
		now,
	).Scan(
		&row.ID,
		&row.UserID,
		&row.ClubID,
		&row.ClubName,
		&row.ClubRole,
		&row.IsActive,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &row, nil
}
