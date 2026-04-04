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

type Claims struct {
	Subject    string `json:"sub"`
	SystemRole string `json:"systemRole"`
	ExpiresAt  int64  `json:"exp"`
}
