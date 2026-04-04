package handlers

import (
	"net/http"
	"strings"

	"pqq/be/internal/auth"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	service *auth.Service
}

func NewAuthHandler(service *auth.Service) *AuthHandler {
	return &AuthHandler{service: service}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var request auth.LoginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	response, err := h.service.Login(c.Request.Context(), request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) Me(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	response, err := h.service.GetMe(c.Request.Context(), claims.Subject)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) Memberships(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	response, err := h.service.ListMemberships(c.Request.Context(), claims.Subject)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": response})
}

func (h *AuthHandler) ClubPermissions(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	clubID := strings.TrimSpace(c.Param("clubId"))
	if clubID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "clubId is required"})
		return
	}

	response, err := h.service.GetClubPermissions(c.Request.Context(), claims.Subject, clubID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) ListUsers(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	response, err := h.service.ListUsers(c.Request.Context(), claims.Subject)
	if err != nil {
		handleAuthServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) CreateUser(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var request auth.CreateUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	response, err := h.service.CreateUser(c.Request.Context(), claims.Subject, request)
	if err != nil {
		handleAuthServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, response)
}

func (h *AuthHandler) UpdateUserStatus(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userID := strings.TrimSpace(c.Param("userId"))
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId is required"})
		return
	}

	var request auth.UpdateUserStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	response, err := h.service.UpdateUserStatus(c.Request.Context(), claims.Subject, userID, request.IsActive)
	if err != nil {
		handleAuthServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) ResetUserPassword(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userID := strings.TrimSpace(c.Param("userId"))
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId is required"})
		return
	}

	var request auth.ResetPasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	response, err := h.service.ResetUserPassword(c.Request.Context(), claims.Subject, userID, request.Password)
	if err != nil {
		handleAuthServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) GetUserMemberships(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userID := strings.TrimSpace(c.Param("userId"))
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId is required"})
		return
	}

	response, err := h.service.GetUserMemberships(c.Request.Context(), claims.Subject, userID)
	if err != nil {
		handleAuthServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) AddMembership(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userID := strings.TrimSpace(c.Param("userId"))
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId is required"})
		return
	}

	var request auth.CreateMembershipRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	response, err := h.service.AddMembership(c.Request.Context(), claims.Subject, userID, request)
	if err != nil {
		handleAuthServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, response)
}

func (h *AuthHandler) ListClubInvites(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	response, err := h.service.ListClubInvites(c.Request.Context(), claims.Subject)
	if err != nil {
		handleAuthServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) CreateClubInvite(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var request auth.CreateClubInviteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	response, err := h.service.CreateClubInvite(c.Request.Context(), claims.Subject, request)
	if err != nil {
		handleAuthServiceError(c, err)
		return
	}
	response.ShareURL = buildClubInviteShareURL(c, response.Token)
	c.JSON(http.StatusCreated, response)
}

func (h *AuthHandler) RevokeClubInvite(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	inviteID := strings.TrimSpace(c.Param("inviteId"))
	if inviteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "inviteId is required"})
		return
	}

	response, err := h.service.RevokeClubInvite(c.Request.Context(), claims.Subject, inviteID)
	if err != nil {
		handleAuthServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) GetClubInvitePreview(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	response, err := h.service.GetClubInvitePreview(c.Request.Context(), token)
	if err != nil {
		handleAuthServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) AcceptClubInvite(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	var request auth.AcceptClubInviteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	response, err := h.service.AcceptClubInvite(c.Request.Context(), token, request)
	if err != nil {
		handleAuthServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) RemoveMembership(c *gin.Context) {
	claims, ok := claimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	membershipID := strings.TrimSpace(c.Param("membershipId"))
	if membershipID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "membershipId is required"})
		return
	}

	response, err := h.service.RemoveMembership(c.Request.Context(), claims.Subject, membershipID)
	if err != nil {
		handleAuthServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func AuthMiddleware(service *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		token := strings.TrimSpace(authHeader[len("Bearer "):])
		claims, err := service.ParseToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		c.Request = c.Request.WithContext(auth.WithClaims(c.Request.Context(), claims))
		c.Set(authClaimsContextKey, claims)
		c.Next()
	}
}

const authClaimsContextKey = "auth_claims"

func claimsFromContext(c *gin.Context) (*auth.Claims, bool) {
	value, ok := c.Get(authClaimsContextKey)
	if !ok {
		return nil, false
	}
	claims, ok := value.(*auth.Claims)
	return claims, ok
}

func handleAuthServiceError(c *gin.Context, err error) {
	switch err.Error() {
	case "forbidden":
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case "user does not exist", "membership does not exist", "invite does not exist":
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case "invalid credentials":
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}

func buildClubInviteShareURL(c *gin.Context, token string) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host + "/accept-invite/" + token
}
