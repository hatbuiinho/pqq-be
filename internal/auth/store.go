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

type clubInviteRow struct {
	ID               string
	ClubID           string
	ClubName         string
	InviterUserID    string
	InviterName      string
	InviteeEmail     *string
	ClubRole         string
	TokenHash        string
	ExpiresAt        time.Time
	MaxUses          int
	UseCount         int
	LastUsedAt       *time.Time
	AcceptedAt       *time.Time
	AcceptedByUserID *string
	RevokedAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
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

func (s *Store) ListActiveClubIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id
		   FROM clubs
		  WHERE deleted_at IS NULL
		  ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]string, 0)
	for rows.Next() {
		var clubID string
		if err := rows.Scan(&clubID); err != nil {
			return nil, err
		}
		result = append(result, clubID)
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

func (s *Store) ListClubInvites(ctx context.Context, clubIDs []string) ([]clubInviteRow, error) {
	if len(clubIDs) == 0 {
		return []clubInviteRow{}, nil
	}

	rows, err := s.pool.Query(
		ctx,
		`SELECT ci.id, ci.club_id, c.name, ci.inviter_user_id, u.full_name, ci.invitee_email,
		        ci.club_role, ci.token_hash, ci.expires_at, ci.max_uses, ci.use_count,
		        ci.last_used_at, ci.accepted_at, ci.accepted_by_user_id, ci.revoked_at,
		        ci.created_at, ci.updated_at
		   FROM club_invites ci
		   INNER JOIN clubs c ON c.id = ci.club_id
		   INNER JOIN users u ON u.id = ci.inviter_user_id
		  WHERE ci.club_id = ANY($1)
		  ORDER BY ci.created_at DESC`,
		clubIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]clubInviteRow, 0)
	for rows.Next() {
		var row clubInviteRow
		if err := rows.Scan(
			&row.ID,
			&row.ClubID,
			&row.ClubName,
			&row.InviterUserID,
			&row.InviterName,
			&row.InviteeEmail,
			&row.ClubRole,
			&row.TokenHash,
			&row.ExpiresAt,
			&row.MaxUses,
			&row.UseCount,
			&row.LastUsedAt,
			&row.AcceptedAt,
			&row.AcceptedByUserID,
			&row.RevokedAt,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}

	return result, rows.Err()
}

func (s *Store) CreateClubInvite(
	ctx context.Context,
	clubID string,
	inviterUserID string,
	inviteeEmail *string,
	clubRole string,
	tokenHash string,
	expiresAt time.Time,
	maxUses int,
) (*clubInviteRow, error) {
	now := time.Now().UTC()
	row := clubInviteRow{}
	err := s.pool.QueryRow(
		ctx,
		`INSERT INTO club_invites (
			id, club_id, inviter_user_id, invitee_email, club_role, token_hash,
			expires_at, max_uses, use_count, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 0, $9, $9)
		RETURNING id, club_id,
			(SELECT name FROM clubs WHERE id = $2),
			inviter_user_id,
			(SELECT full_name FROM users WHERE id = $3),
			invitee_email, club_role, token_hash, expires_at, max_uses, use_count,
			last_used_at, accepted_at, accepted_by_user_id, revoked_at, created_at, updated_at`,
		uuid.NewString(),
		clubID,
		inviterUserID,
		inviteeEmail,
		clubRole,
		tokenHash,
		expiresAt,
		maxUses,
		now,
	).Scan(
		&row.ID,
		&row.ClubID,
		&row.ClubName,
		&row.InviterUserID,
		&row.InviterName,
		&row.InviteeEmail,
		&row.ClubRole,
		&row.TokenHash,
		&row.ExpiresAt,
		&row.MaxUses,
		&row.UseCount,
		&row.LastUsedAt,
		&row.AcceptedAt,
		&row.AcceptedByUserID,
		&row.RevokedAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &row, nil
}

func (s *Store) FindClubInviteByID(ctx context.Context, inviteID string) (*clubInviteRow, error) {
	row := clubInviteRow{}
	err := s.pool.QueryRow(
		ctx,
		`SELECT ci.id, ci.club_id, c.name, ci.inviter_user_id, u.full_name, ci.invitee_email,
		        ci.club_role, ci.token_hash, ci.expires_at, ci.max_uses, ci.use_count,
		        ci.last_used_at, ci.accepted_at, ci.accepted_by_user_id, ci.revoked_at,
		        ci.created_at, ci.updated_at
		   FROM club_invites ci
		   INNER JOIN clubs c ON c.id = ci.club_id
		   INNER JOIN users u ON u.id = ci.inviter_user_id
		  WHERE ci.id = $1
		  LIMIT 1`,
		inviteID,
	).Scan(
		&row.ID,
		&row.ClubID,
		&row.ClubName,
		&row.InviterUserID,
		&row.InviterName,
		&row.InviteeEmail,
		&row.ClubRole,
		&row.TokenHash,
		&row.ExpiresAt,
		&row.MaxUses,
		&row.UseCount,
		&row.LastUsedAt,
		&row.AcceptedAt,
		&row.AcceptedByUserID,
		&row.RevokedAt,
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

func (s *Store) FindActiveClubInviteByTokenHash(ctx context.Context, tokenHash string) (*clubInviteRow, error) {
	row := clubInviteRow{}
	err := s.pool.QueryRow(
		ctx,
		`SELECT ci.id, ci.club_id, c.name, ci.inviter_user_id, u.full_name, ci.invitee_email,
		        ci.club_role, ci.token_hash, ci.expires_at, ci.max_uses, ci.use_count,
		        ci.last_used_at, ci.accepted_at, ci.accepted_by_user_id, ci.revoked_at,
		        ci.created_at, ci.updated_at
		   FROM club_invites ci
		   INNER JOIN clubs c ON c.id = ci.club_id
		   INNER JOIN users u ON u.id = ci.inviter_user_id
		  WHERE ci.token_hash = $1
		    AND ci.revoked_at IS NULL
		    AND ci.accepted_at IS NULL
		  LIMIT 1`,
		tokenHash,
	).Scan(
		&row.ID,
		&row.ClubID,
		&row.ClubName,
		&row.InviterUserID,
		&row.InviterName,
		&row.InviteeEmail,
		&row.ClubRole,
		&row.TokenHash,
		&row.ExpiresAt,
		&row.MaxUses,
		&row.UseCount,
		&row.LastUsedAt,
		&row.AcceptedAt,
		&row.AcceptedByUserID,
		&row.RevokedAt,
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

func (s *Store) RevokeClubInvite(ctx context.Context, inviteID string) (*clubInviteRow, error) {
	row := clubInviteRow{}
	now := time.Now().UTC()
	err := s.pool.QueryRow(
		ctx,
		`UPDATE club_invites ci
		    SET revoked_at = $2,
		        updated_at = $2
		   FROM clubs c, users u
		  WHERE ci.id = $1
		    AND ci.club_id = c.id
		    AND ci.inviter_user_id = u.id
		    AND ci.revoked_at IS NULL
		    AND ci.accepted_at IS NULL
		RETURNING ci.id, ci.club_id, c.name, ci.inviter_user_id, u.full_name, ci.invitee_email,
		          ci.club_role, ci.token_hash, ci.expires_at, ci.max_uses, ci.use_count,
		          ci.last_used_at, ci.accepted_at, ci.accepted_by_user_id, ci.revoked_at,
		          ci.created_at, ci.updated_at`,
		inviteID,
		now,
	).Scan(
		&row.ID,
		&row.ClubID,
		&row.ClubName,
		&row.InviterUserID,
		&row.InviterName,
		&row.InviteeEmail,
		&row.ClubRole,
		&row.TokenHash,
		&row.ExpiresAt,
		&row.MaxUses,
		&row.UseCount,
		&row.LastUsedAt,
		&row.AcceptedAt,
		&row.AcceptedByUserID,
		&row.RevokedAt,
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

func (s *Store) AcceptClubInvite(
	ctx context.Context,
	inviteID string,
	acceptedByUserID string,
) (*clubInviteRow, error) {
	row := clubInviteRow{}
	now := time.Now().UTC()
	err := s.pool.QueryRow(
		ctx,
		`UPDATE club_invites ci
		    SET use_count = ci.use_count + 1,
		        last_used_at = $3,
		        accepted_at = CASE WHEN ci.use_count + 1 >= ci.max_uses THEN $3 ELSE ci.accepted_at END,
		        accepted_by_user_id = CASE WHEN ci.use_count + 1 >= ci.max_uses THEN $2 ELSE ci.accepted_by_user_id END,
		        updated_at = $3
		   FROM clubs c, users u
		  WHERE ci.id = $1
		    AND ci.club_id = c.id
		    AND ci.inviter_user_id = u.id
		RETURNING ci.id, ci.club_id, c.name, ci.inviter_user_id, u.full_name, ci.invitee_email,
		          ci.club_role, ci.token_hash, ci.expires_at, ci.max_uses, ci.use_count,
		          ci.last_used_at, ci.accepted_at, ci.accepted_by_user_id, ci.revoked_at,
		          ci.created_at, ci.updated_at`,
		inviteID,
		acceptedByUserID,
		now,
	).Scan(
		&row.ID,
		&row.ClubID,
		&row.ClubName,
		&row.InviterUserID,
		&row.InviterName,
		&row.InviteeEmail,
		&row.ClubRole,
		&row.TokenHash,
		&row.ExpiresAt,
		&row.MaxUses,
		&row.UseCount,
		&row.LastUsedAt,
		&row.AcceptedAt,
		&row.AcceptedByUserID,
		&row.RevokedAt,
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
