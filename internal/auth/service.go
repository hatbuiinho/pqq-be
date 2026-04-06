package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	TokenSecret            string
	TokenTTLMinutes        int
	BootstrapAdminEmail    string
	BootstrapAdminName     string
	BootstrapAdminPassword string
}

type Service struct {
	store           *Store
	tokenSecret     []byte
	tokenTTL        time.Duration
	bootstrapConfig Config
}

func NewService(store *Store, cfg Config) *Service {
	tokenTTL := time.Duration(cfg.TokenTTLMinutes) * time.Minute
	if tokenTTL <= 0 {
		tokenTTL = 12 * time.Hour
	}

	return &Service{
		store:           store,
		tokenSecret:     []byte(cfg.TokenSecret),
		tokenTTL:        tokenTTL,
		bootstrapConfig: cfg,
	}
}

func (s *Service) EnsureBootstrapSysAdmin(ctx context.Context) error {
	if strings.TrimSpace(s.bootstrapConfig.BootstrapAdminEmail) == "" {
		return nil
	}

	passwordHash, err := HashPassword(s.bootstrapConfig.BootstrapAdminPassword)
	if err != nil {
		return err
	}

	return s.store.EnsureBootstrapSysAdmin(
		ctx,
		s.bootstrapConfig.BootstrapAdminEmail,
		strings.TrimSpace(s.bootstrapConfig.BootstrapAdminName),
		passwordHash,
	)
}

func (s *Service) Login(ctx context.Context, request LoginRequest) (*LoginResponse, error) {
	email := strings.TrimSpace(request.Email)
	password := request.Password
	if email == "" || password == "" {
		return nil, errors.New("vui lòng nhập email và mật khẩu")
	}

	userRow, err := s.store.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if userRow == nil {
		return nil, errors.New("email hoặc mật khẩu không đúng")
	}
	if !userRow.IsActive {
		return nil, errors.New("tài khoản của bạn đã bị khóa")
	}
	if err := VerifyPassword(userRow.PasswordHash, password); err != nil {
		return nil, errors.New("email hoặc mật khẩu không đúng")
	}

	now := time.Now().UTC()
	if err := s.store.UpdateLastLoginAt(ctx, userRow.ID, now); err != nil {
		return nil, err
	}

	userRow.LastLoginAt = &now
	userRow.UpdatedAt = now
	user := mapUser(userRow)
	memberships, err := s.listMemberships(ctx, userRow.ID)
	if err != nil {
		return nil, err
	}

	token, err := s.signClaims(Claims{
		Subject:    userRow.ID,
		SystemRole: userRow.SystemRole,
		ExpiresAt:  now.Add(s.tokenTTL).Unix(),
	})
	if err != nil {
		return nil, err
	}

	return &LoginResponse{
		Token:       token,
		User:        user,
		Memberships: memberships,
	}, nil
}

func (s *Service) GetMe(ctx context.Context, userID string) (*MeResponse, error) {
	userRow, err := s.store.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userRow == nil || !userRow.IsActive {
		return nil, errors.New("tài khoản không tồn tại hoặc đã bị khóa")
	}

	memberships, err := s.listMemberships(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &MeResponse{
		User:        mapUser(userRow),
		Memberships: memberships,
	}, nil
}

func (s *Service) ListMemberships(ctx context.Context, userID string) ([]ClubMembership, error) {
	userRow, err := s.store.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userRow == nil || !userRow.IsActive {
		return nil, errors.New("tài khoản không tồn tại hoặc đã bị khóa")
	}

	return s.listMemberships(ctx, userID)
}

func (s *Service) ListUsers(ctx context.Context, actorUserID string) (*ListUsersResponse, error) {
	if err := s.requireSysAdmin(ctx, actorUserID); err != nil {
		return nil, err
	}

	rows, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]User, 0, len(rows))
	for i := range rows {
		items = append(items, mapUser(&rows[i]))
	}

	return &ListUsersResponse{Items: items}, nil
}

func (s *Service) CreateUser(
	ctx context.Context,
	actorUserID string,
	request CreateUserRequest,
) (*User, error) {
	if err := s.requireSysAdmin(ctx, actorUserID); err != nil {
		return nil, err
	}

	email := strings.TrimSpace(strings.ToLower(request.Email))
	fullName := strings.TrimSpace(request.FullName)
	systemRole := strings.TrimSpace(request.SystemRole)
	if email == "" || fullName == "" || request.Password == "" {
		return nil, errors.New("email, fullName and password are required")
	}
	if systemRole == "" {
		systemRole = SystemRoleUser
	}
	if systemRole != SystemRoleUser && systemRole != SystemRoleSysAdmin {
		return nil, errors.New("invalid system role")
	}

	existingUser, err := s.store.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existingUser != nil {
		return nil, errors.New("user email already exists")
	}

	passwordHash, err := HashPassword(request.Password)
	if err != nil {
		return nil, err
	}

	row, err := s.store.CreateUser(ctx, email, fullName, passwordHash, systemRole, request.IsActive)
	if err != nil {
		return nil, err
	}

	user := mapUser(row)
	if err := s.writeAuditLog(
		ctx,
		actorUserID,
		nil,
		"users",
		user.ID,
		"create",
		nil,
		user,
		map[string]any{
			"systemRole": user.SystemRole,
			"isActive":   user.IsActive,
		},
	); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) UpdateUserStatus(
	ctx context.Context,
	actorUserID string,
	userID string,
	isActive bool,
) (*User, error) {
	if err := s.requireSysAdmin(ctx, actorUserID); err != nil {
		return nil, err
	}
	if actorUserID == userID && !isActive {
		return nil, errors.New("you cannot deactivate your own account")
	}

	existingUserRow, err := s.store.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if existingUserRow == nil {
		return nil, errors.New("user does not exist")
	}

	row, err := s.store.UpdateUserActiveStatus(ctx, userID, isActive)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errors.New("user does not exist")
	}

	user := mapUser(row)
	if err := s.writeAuditLog(
		ctx,
		actorUserID,
		nil,
		"users",
		user.ID,
		"update_status",
		mapUser(existingUserRow),
		user,
		map[string]any{
			"isActive": user.IsActive,
		},
	); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) ResetUserPassword(
	ctx context.Context,
	actorUserID string,
	userID string,
	password string,
) (*User, error) {
	if err := s.requireSysAdmin(ctx, actorUserID); err != nil {
		return nil, err
	}

	passwordHash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	row, err := s.store.UpdateUserPassword(ctx, userID, passwordHash)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errors.New("user does not exist")
	}

	user := mapUser(row)
	if err := s.writeAuditLog(
		ctx,
		actorUserID,
		nil,
		"users",
		user.ID,
		"reset_password",
		nil,
		nil,
		map[string]any{
			"targetUserId": user.ID,
		},
	); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) GetUserMemberships(
	ctx context.Context,
	actorUserID string,
	userID string,
) (*UserMembershipsResponse, error) {
	if err := s.requireSysAdmin(ctx, actorUserID); err != nil {
		return nil, err
	}

	userRow, err := s.store.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userRow == nil {
		return nil, errors.New("user does not exist")
	}

	memberships, err := s.listMemberships(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &UserMembershipsResponse{
		User:        mapUser(userRow),
		Memberships: memberships,
	}, nil
}

func (s *Service) AddMembership(
	ctx context.Context,
	actorUserID string,
	userID string,
	request CreateMembershipRequest,
) (*ClubMembership, error) {
	if err := s.requireSysAdmin(ctx, actorUserID); err != nil {
		return nil, err
	}

	clubID := strings.TrimSpace(request.ClubID)
	clubRole := strings.TrimSpace(request.ClubRole)
	if clubID == "" || clubRole == "" {
		return nil, errors.New("clubId and clubRole are required")
	}
	if clubRole != ClubRoleOwner && clubRole != ClubRoleAssistant {
		return nil, errors.New("invalid club role")
	}

	userRow, err := s.store.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userRow == nil {
		return nil, errors.New("user does not exist")
	}

	existingMembership, err := s.store.FindMembershipByUserAndClub(ctx, userID, clubID)
	if err != nil {
		return nil, err
	}
	if existingMembership != nil {
		return nil, errors.New("membership already exists")
	}

	row, err := s.store.CreateMembership(ctx, userID, clubID, clubRole, request.IsActive)
	if err != nil {
		return nil, err
	}

	membership := mapMembership(row)
	if err := s.writeAuditLog(
		ctx,
		actorUserID,
		&membership.ClubID,
		"club_memberships",
		membership.ID,
		"create",
		nil,
		membership,
		map[string]any{
			"userId":   membership.UserID,
			"clubRole": membership.ClubRole,
			"isActive": membership.IsActive,
		},
	); err != nil {
		return nil, err
	}
	return &membership, nil
}

func (s *Service) ListClubInvites(
	ctx context.Context,
	actorUserID string,
) (*ListClubInvitesResponse, error) {
	userRow, err := s.store.FindUserByID(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if userRow == nil || !userRow.IsActive {
		return nil, errors.New("user does not exist")
	}

	clubIDs, err := s.accessibleInviteClubIDs(ctx, actorUserID, userRow.SystemRole)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.ListClubInvites(ctx, clubIDs)
	if err != nil {
		return nil, err
	}

	items := make([]ClubInvite, 0, len(rows))
	for i := range rows {
		items = append(items, mapClubInvite(&rows[i]))
	}
	return &ListClubInvitesResponse{Items: items}, nil
}

func (s *Service) CreateClubInvite(
	ctx context.Context,
	actorUserID string,
	request CreateClubInviteRequest,
) (*CreateClubInviteResponse, error) {
	userRow, err := s.store.FindUserByID(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if userRow == nil || !userRow.IsActive {
		return nil, errors.New("user does not exist")
	}

	clubID := strings.TrimSpace(request.ClubID)
	clubRole := strings.TrimSpace(request.ClubRole)
	if clubID == "" || clubRole == "" {
		return nil, errors.New("clubId and clubRole are required")
	}
	if clubRole != ClubRoleOwner && clubRole != ClubRoleAssistant {
		return nil, errors.New("invalid club role")
	}

	if userRow.SystemRole != SystemRoleSysAdmin {
		membershipRow, err := s.store.FindMembershipByUserAndClub(ctx, actorUserID, clubID)
		if err != nil {
			return nil, err
		}
		if membershipRow == nil || !membershipRow.IsActive || membershipRow.ClubRole != ClubRoleOwner {
			return nil, errors.New("forbidden")
		}
		if clubRole != ClubRoleAssistant {
			return nil, errors.New("owners can only create assistant invite links")
		}
	}

	expiresInDays := request.ExpiresInDays
	if expiresInDays <= 0 {
		expiresInDays = 3
	}
	if expiresInDays > 30 {
		return nil, errors.New("expiresInDays must be between 1 and 30")
	}

	rawToken, tokenHash, err := generateInviteToken()
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().UTC().Add(time.Duration(expiresInDays) * 24 * time.Hour)
	var inviteeEmail *string
	if email := strings.TrimSpace(strings.ToLower(request.InviteeEmail)); email != "" {
		inviteeEmail = &email
	}

	row, err := s.store.CreateClubInvite(
		ctx,
		clubID,
		actorUserID,
		inviteeEmail,
		clubRole,
		tokenHash,
		expiresAt,
		1,
	)
	if err != nil {
		return nil, err
	}

	invite := mapClubInvite(row)
	if err := s.writeAuditLog(
		ctx,
		actorUserID,
		&invite.ClubID,
		"club_invites",
		invite.ID,
		"create",
		nil,
		invite,
		map[string]any{
			"clubRole":      invite.ClubRole,
			"inviteeEmail":  invite.InviteeEmail,
			"expiresAt":     invite.ExpiresAt,
			"maxUses":       invite.MaxUses,
			"shareMode":     "link",
			"createdByRole": userRow.SystemRole,
		},
	); err != nil {
		return nil, err
	}

	return &CreateClubInviteResponse{
		Invite: invite,
		Token:  rawToken,
	}, nil
}

func (s *Service) RevokeClubInvite(
	ctx context.Context,
	actorUserID string,
	inviteID string,
) (*ClubInvite, error) {
	userRow, err := s.store.FindUserByID(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if userRow == nil || !userRow.IsActive {
		return nil, errors.New("user does not exist")
	}

	inviteRow, err := s.store.FindClubInviteByID(ctx, inviteID)
	if err != nil {
		return nil, err
	}
	if inviteRow == nil {
		return nil, errors.New("invite does not exist")
	}
	if err := s.requireInviteAccess(ctx, actorUserID, userRow.SystemRole, inviteRow, true); err != nil {
		return nil, err
	}

	row, err := s.store.RevokeClubInvite(ctx, inviteID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errors.New("invite does not exist")
	}

	invite := mapClubInvite(row)
	if err := s.writeAuditLog(
		ctx,
		actorUserID,
		&invite.ClubID,
		"club_invites",
		invite.ID,
		"revoke",
		mapClubInvite(inviteRow),
		invite,
		map[string]any{
			"inviteeEmail": invite.InviteeEmail,
		},
	); err != nil {
		return nil, err
	}
	return &invite, nil
}

func (s *Service) GetClubInvitePreview(
	ctx context.Context,
	token string,
) (*ClubInvitePreview, error) {
	row, err := s.store.FindActiveClubInviteByTokenHash(ctx, hashInviteToken(token))
	if err != nil {
		return nil, err
	}
	if row == nil || row.RevokedAt != nil {
		return nil, errors.New("invite does not exist")
	}
	if err := validateInviteActive(row); err != nil {
		return nil, err
	}

	return &ClubInvitePreview{
		ClubID:       row.ClubID,
		ClubName:     row.ClubName,
		ClubRole:     row.ClubRole,
		InviterName:  row.InviterName,
		InviteeEmail: row.InviteeEmail,
		ExpiresAt:    row.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) AcceptClubInvite(
	ctx context.Context,
	token string,
	request AcceptClubInviteRequest,
) (*LoginResponse, error) {
	row, err := s.store.FindActiveClubInviteByTokenHash(ctx, hashInviteToken(token))
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errors.New("invite does not exist")
	}
	if err := validateInviteActive(row); err != nil {
		return nil, err
	}

	email := strings.TrimSpace(strings.ToLower(request.Email))
	password := request.Password
	if email == "" || password == "" {
		return nil, errors.New("email and password are required")
	}
	if row.InviteeEmail != nil && strings.TrimSpace(strings.ToLower(*row.InviteeEmail)) != email {
		return nil, errors.New("invite email does not match")
	}

	userRow, err := s.store.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if userRow != nil && !userRow.IsActive {
		return nil, errors.New("account is inactive")
	}

	if userRow == nil {
		fullName := strings.TrimSpace(request.FullName)
		if fullName == "" {
			return nil, errors.New("fullName is required")
		}
		passwordHash, err := HashPassword(password)
		if err != nil {
			return nil, err
		}
		userRow, err = s.store.CreateUser(ctx, email, fullName, passwordHash, SystemRoleUser, true)
		if err != nil {
			return nil, err
		}
	} else if err := VerifyPassword(userRow.PasswordHash, password); err != nil {
		return nil, errors.New("invalid credentials")
	}

	existingMembership, err := s.store.FindMembershipByUserAndClub(ctx, userRow.ID, row.ClubID)
	if err != nil {
		return nil, err
	}
	if existingMembership == nil {
		createdMembership, err := s.store.CreateMembership(ctx, userRow.ID, row.ClubID, row.ClubRole, true)
		if err != nil {
			return nil, err
		}
		membership := mapMembership(createdMembership)
		if err := s.writeAuditLog(
			ctx,
			userRow.ID,
			&membership.ClubID,
			"club_memberships",
			membership.ID,
			"create_from_invite",
			nil,
			membership,
			map[string]any{
				"userId":   membership.UserID,
				"clubRole": membership.ClubRole,
				"source":   "invite_accept",
			},
		); err != nil {
			return nil, err
		}
	}

	acceptedInviteRow, err := s.store.AcceptClubInvite(ctx, row.ID, userRow.ID)
	if err != nil {
		return nil, err
	}
	if acceptedInviteRow == nil {
		return nil, errors.New("invite does not exist")
	}
	acceptedInvite := mapClubInvite(acceptedInviteRow)
	if err := s.writeAuditLog(
		ctx,
		userRow.ID,
		&acceptedInvite.ClubID,
		"club_invites",
		acceptedInvite.ID,
		"accept",
		mapClubInvite(row),
		acceptedInvite,
		map[string]any{
			"acceptedByUserId": userRow.ID,
			"acceptedByEmail":  userRow.Email,
		},
	); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if err := s.store.UpdateLastLoginAt(ctx, userRow.ID, now); err != nil {
		return nil, err
	}
	userRow.LastLoginAt = &now
	userRow.UpdatedAt = now

	memberships, err := s.listMemberships(ctx, userRow.ID)
	if err != nil {
		return nil, err
	}
	tokenValue, err := s.signClaims(Claims{
		Subject:    userRow.ID,
		SystemRole: userRow.SystemRole,
		ExpiresAt:  now.Add(s.tokenTTL).Unix(),
	})
	if err != nil {
		return nil, err
	}

	return &LoginResponse{
		Token:       tokenValue,
		User:        mapUser(userRow),
		Memberships: memberships,
	}, nil
}

func (s *Service) RemoveMembership(
	ctx context.Context,
	actorUserID string,
	membershipID string,
) (*ClubMembership, error) {
	if err := s.requireSysAdmin(ctx, actorUserID); err != nil {
		return nil, err
	}

	row, err := s.store.RevokeMembership(ctx, membershipID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errors.New("membership does not exist")
	}

	membership := mapMembership(row)
	if err := s.writeAuditLog(
		ctx,
		actorUserID,
		&membership.ClubID,
		"club_memberships",
		membership.ID,
		"revoke",
		membership,
		nil,
		map[string]any{
			"userId": membership.UserID,
		},
	); err != nil {
		return nil, err
	}
	return &membership, nil
}

func (s *Service) GetClubPermissions(
	ctx context.Context,
	userID string,
	clubID string,
) (*ClubPermissionResponse, error) {
	userRow, err := s.store.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if userRow == nil || !userRow.IsActive {
		return nil, errors.New("user does not exist")
	}

	if userRow.SystemRole == SystemRoleSysAdmin {
		return &ClubPermissionResponse{
			ClubID:       clubID,
			SystemRole:   userRow.SystemRole,
			IsSystemRole: true,
			Permissions:  EvaluatePermissions(userRow.SystemRole, ""),
		}, nil
	}

	membershipRow, err := s.store.FindMembershipByUserAndClub(ctx, userID, clubID)
	if err != nil {
		return nil, err
	}
	if membershipRow == nil || !membershipRow.IsActive {
		return &ClubPermissionResponse{
			ClubID:       clubID,
			SystemRole:   userRow.SystemRole,
			IsSystemRole: false,
			Permissions:  EvaluatePermissions(userRow.SystemRole, ""),
		}, nil
	}

	return &ClubPermissionResponse{
		ClubID:       clubID,
		SystemRole:   userRow.SystemRole,
		ClubRole:     membershipRow.ClubRole,
		IsSystemRole: false,
		Permissions:  EvaluatePermissions(userRow.SystemRole, membershipRow.ClubRole),
	}, nil
}

func (s *Service) ListAuditLogs(
	ctx context.Context,
	actorUserID string,
	query ListAuditLogsQuery,
) (*ListAuditLogsResponse, error) {
	userRow, err := s.store.FindUserByID(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if userRow == nil || !userRow.IsActive {
		return nil, errors.New("user does not exist")
	}

	var allowedClubIDs []string
	if userRow.SystemRole != SystemRoleSysAdmin {
		if strings.TrimSpace(query.ClubID) == "" {
			return nil, errors.New("clubId is required")
		}

		membershipRow, err := s.store.FindMembershipByUserAndClub(ctx, actorUserID, query.ClubID)
		if err != nil {
			return nil, err
		}
		if membershipRow == nil || !membershipRow.IsActive || membershipRow.ClubRole != ClubRoleOwner {
			return nil, errors.New("forbidden")
		}
		allowedClubIDs = []string{membershipRow.ClubID}
	}

	rows, err := s.store.ListAuditLogs(ctx, query, allowedClubIDs)
	if err != nil {
		return nil, err
	}

	items := make([]AuditLog, 0, len(rows))
	for i := range rows {
		items = append(items, mapAuditLog(&rows[i]))
	}
	return &ListAuditLogsResponse{Items: items}, nil
}

func (s *Service) ParseToken(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid token")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("invalid token")
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid token")
	}

	expected := s.computeSignature(payload)
	if !hmac.Equal(signature, expected) {
		return nil, errors.New("invalid token")
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errors.New("invalid token")
	}
	if claims.Subject == "" || claims.ExpiresAt <= time.Now().UTC().Unix() {
		return nil, errors.New("token expired")
	}

	return &claims, nil
}

func HashPassword(password string) (string, error) {
	trimmed := strings.TrimSpace(password)
	if len(trimmed) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(trimmed), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(passwordHash string, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
}

func (s *Service) signClaims(claims Claims) (string, error) {
	if len(s.tokenSecret) == 0 {
		return "", errors.New("auth token secret is not configured")
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signature := s.computeSignature(payload)
	return fmt.Sprintf(
		"%s.%s",
		base64.RawURLEncoding.EncodeToString(payload),
		base64.RawURLEncoding.EncodeToString(signature),
	), nil
}

func (s *Service) computeSignature(payload []byte) []byte {
	mac := hmac.New(sha256.New, s.tokenSecret)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func (s *Service) listMemberships(ctx context.Context, userID string) ([]ClubMembership, error) {
	rows, err := s.store.ListMembershipsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]ClubMembership, 0, len(rows))
	for i := range rows {
		result = append(result, mapMembership(&rows[i]))
	}
	return result, nil
}

func mapUser(row *userRow) User {
	return User{
		ID:          row.ID,
		Email:       row.Email,
		FullName:    row.FullName,
		SystemRole:  row.SystemRole,
		IsActive:    row.IsActive,
		LastLoginAt: formatOptionalTime(row.LastLoginAt),
		CreatedAt:   row.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:   row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func mapMembership(row *membershipRow) ClubMembership {
	return ClubMembership{
		ID:        row.ID,
		UserID:    row.UserID,
		ClubID:    row.ClubID,
		ClubName:  row.ClubName,
		ClubRole:  row.ClubRole,
		IsActive:  row.IsActive,
		CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func mapClubInvite(row *clubInviteRow) ClubInvite {
	return ClubInvite{
		ID:               row.ID,
		ClubID:           row.ClubID,
		ClubName:         row.ClubName,
		InviterUserID:    row.InviterUserID,
		InviterName:      row.InviterName,
		InviteeEmail:     row.InviteeEmail,
		ClubRole:         row.ClubRole,
		ExpiresAt:        row.ExpiresAt.UTC().Format(time.RFC3339Nano),
		MaxUses:          row.MaxUses,
		UseCount:         row.UseCount,
		LastUsedAt:       formatOptionalTime(row.LastUsedAt),
		AcceptedAt:       formatOptionalTime(row.AcceptedAt),
		AcceptedByUserID: row.AcceptedByUserID,
		AcceptedByName:   row.AcceptedByName,
		AcceptedByEmail:  row.AcceptedByEmail,
		RevokedAt:        formatOptionalTime(row.RevokedAt),
		CreatedAt:        row.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:        row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func mapAuditLog(row *auditLogRow) AuditLog {
	return AuditLog{
		ID:          row.ID,
		ActorUserID: row.ActorUserID,
		ActorName:   row.ActorName,
		ClubID:      row.ClubID,
		EntityType:  row.EntityType,
		EntityID:    row.EntityID,
		Action:      row.Action,
		OldValues:   row.OldValues,
		NewValues:   row.NewValues,
		Metadata:    row.Metadata,
		CreatedAt:   row.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (s *Service) writeAuditLog(
	ctx context.Context,
	actorUserID string,
	clubID *string,
	entityType string,
	entityID string,
	action string,
	oldValue any,
	newValue any,
	metadata map[string]any,
) error {
	oldValues, err := marshalAuditValue(oldValue)
	if err != nil {
		return err
	}
	newValues, err := marshalAuditValue(newValue)
	if err != nil {
		return err
	}
	metadataValue, err := marshalAuditMetadata(metadata)
	if err != nil {
		return err
	}

	var actorID *string
	if trimmed := strings.TrimSpace(actorUserID); trimmed != "" {
		actorID = &trimmed
	}

	var entityIDPtr *string
	if trimmed := strings.TrimSpace(entityID); trimmed != "" {
		entityIDPtr = &trimmed
	}

	return s.store.InsertAuditLog(
		ctx,
		actorID,
		clubID,
		entityType,
		entityIDPtr,
		action,
		oldValues,
		newValues,
		metadataValue,
	)
}

func marshalAuditValue(value any) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func marshalAuditMetadata(metadata map[string]any) (json.RawMessage, error) {
	if metadata == nil {
		return json.RawMessage(`{}`), nil
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func (s *Service) requireSysAdmin(ctx context.Context, userID string) error {
	userRow, err := s.store.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if userRow == nil || !userRow.IsActive {
		return errors.New("user does not exist")
	}
	if userRow.SystemRole != SystemRoleSysAdmin {
		return errors.New("forbidden")
	}
	return nil
}

func (s *Service) accessibleInviteClubIDs(
	ctx context.Context,
	userID string,
	systemRole string,
) ([]string, error) {
	if systemRole == SystemRoleSysAdmin {
		return s.store.ListActiveClubIDs(ctx)
	}

	memberships, err := s.store.ListMembershipsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	clubIDs := make([]string, 0)
	for _, membership := range memberships {
		if membership.IsActive && membership.ClubRole == ClubRoleOwner {
			clubIDs = append(clubIDs, membership.ClubID)
		}
	}
	return clubIDs, nil
}

func (s *Service) requireInviteAccess(
	ctx context.Context,
	actorUserID string,
	systemRole string,
	invite *clubInviteRow,
	forWrite bool,
) error {
	if systemRole == SystemRoleSysAdmin {
		return nil
	}
	membershipRow, err := s.store.FindMembershipByUserAndClub(ctx, actorUserID, invite.ClubID)
	if err != nil {
		return err
	}
	if membershipRow == nil || !membershipRow.IsActive || membershipRow.ClubRole != ClubRoleOwner {
		return errors.New("forbidden")
	}
	if forWrite && invite.ClubRole != ClubRoleAssistant {
		return errors.New("forbidden")
	}
	return nil
}

func validateInviteActive(row *clubInviteRow) error {
	now := time.Now().UTC()
	if row.RevokedAt != nil {
		return errors.New("invite has been revoked")
	}
	if row.AcceptedAt != nil {
		return errors.New("invite has already been used")
	}
	if row.ExpiresAt.Before(now) {
		return errors.New("invite has expired")
	}
	if row.UseCount >= row.MaxUses {
		return errors.New("invite has already been used")
	}
	return nil
}

func generateInviteToken() (string, string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", "", err
	}
	rawToken := hex.EncodeToString(buffer)
	return rawToken, hashInviteToken(rawToken), nil
}

func hashInviteToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}
