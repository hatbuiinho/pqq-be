package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

	row, err := s.store.UpdateUserActiveStatus(ctx, userID, isActive)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errors.New("user does not exist")
	}

	user := mapUser(row)
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
	return &membership, nil
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
