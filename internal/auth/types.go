package auth

type User struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	FullName    string  `json:"fullName"`
	SystemRole  string  `json:"systemRole"`
	IsActive    bool    `json:"isActive"`
	LastLoginAt *string `json:"lastLoginAt,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

type ClubMembership struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	ClubID    string `json:"clubId"`
	ClubName  string `json:"clubName"`
	ClubRole  string `json:"clubRole"`
	IsActive  bool   `json:"isActive"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token       string           `json:"token"`
	User        User             `json:"user"`
	Memberships []ClubMembership `json:"memberships"`
}

type MeResponse struct {
	User        User             `json:"user"`
	Memberships []ClubMembership `json:"memberships"`
}

type ClubPermissionResponse struct {
	ClubID       string          `json:"clubId"`
	SystemRole   string          `json:"systemRole"`
	ClubRole     string          `json:"clubRole,omitempty"`
	IsSystemRole bool            `json:"isSystemRole"`
	Permissions  map[string]bool `json:"permissions"`
}

type ListUsersResponse struct {
	Items []User `json:"items"`
}

type CreateUserRequest struct {
	Email      string `json:"email"`
	FullName   string `json:"fullName"`
	Password   string `json:"password"`
	SystemRole string `json:"systemRole"`
	IsActive   bool   `json:"isActive"`
}

type UpdateUserStatusRequest struct {
	IsActive bool `json:"isActive"`
}

type ResetPasswordRequest struct {
	Password string `json:"password"`
}

type UserMembershipsResponse struct {
	User        User             `json:"user"`
	Memberships []ClubMembership `json:"memberships"`
}

type CreateMembershipRequest struct {
	ClubID   string `json:"clubId"`
	ClubRole string `json:"clubRole"`
	IsActive bool   `json:"isActive"`
}

type ClubInvite struct {
	ID               string  `json:"id"`
	ClubID           string  `json:"clubId"`
	ClubName         string  `json:"clubName"`
	InviterUserID    string  `json:"inviterUserId"`
	InviterName      string  `json:"inviterName"`
	InviteeEmail     *string `json:"inviteeEmail,omitempty"`
	ClubRole         string  `json:"clubRole"`
	ExpiresAt        string  `json:"expiresAt"`
	MaxUses          int     `json:"maxUses"`
	UseCount         int     `json:"useCount"`
	LastUsedAt       *string `json:"lastUsedAt,omitempty"`
	AcceptedAt       *string `json:"acceptedAt,omitempty"`
	AcceptedByUserID *string `json:"acceptedByUserId,omitempty"`
	RevokedAt        *string `json:"revokedAt,omitempty"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        string  `json:"updatedAt"`
}

type ListClubInvitesResponse struct {
	Items []ClubInvite `json:"items"`
}

type CreateClubInviteResponse struct {
	Invite   ClubInvite `json:"invite"`
	Token    string     `json:"token"`
	ShareURL string     `json:"shareUrl"`
}

type CreateClubInviteRequest struct {
	ClubID        string `json:"clubId"`
	ClubRole      string `json:"clubRole"`
	InviteeEmail  string `json:"inviteeEmail,omitempty"`
	ExpiresInDays int    `json:"expiresInDays"`
}

type ClubInvitePreview struct {
	ClubID       string  `json:"clubId"`
	ClubName     string  `json:"clubName"`
	ClubRole     string  `json:"clubRole"`
	InviterName  string  `json:"inviterName"`
	InviteeEmail *string `json:"inviteeEmail,omitempty"`
	ExpiresAt    string  `json:"expiresAt"`
}

type AcceptClubInviteRequest struct {
	Email    string `json:"email"`
	FullName string `json:"fullName,omitempty"`
	Password string `json:"password"`
}

type Claims struct {
	Subject    string `json:"sub"`
	SystemRole string `json:"systemRole"`
	ExpiresAt  int64  `json:"exp"`
}
