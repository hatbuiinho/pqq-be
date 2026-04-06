package media

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"pqq/be/internal/auth"
	"pqq/be/internal/storage"

	"github.com/disintegration/imaging"
	"github.com/jackc/pgx/v5"
	_ "golang.org/x/image/webp"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const (
	mediaTypeAvatar               = "avatar"
	avatarSourceManual            = "manual"
	avatarSourceBatchImport       = "batch_import"
	maxAvatarSize           int64 = 5 * 1024 * 1024
	avatarThumbSize               = 160
	avatarOriginalCache           = "private, max-age=300"
	avatarThumbnailCache          = "public, max-age=31536000, immutable"
)

var avatarMimeTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

var unsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type Service struct {
	store         *Store
	storage       storage.Service
	presignExpiry time.Duration
	authorizer    Authorizer
}

type Authorizer interface {
	GetClubPermissions(ctx context.Context, userID string, clubID string) (*auth.ClubPermissionResponse, error)
	ListMemberships(ctx context.Context, userID string) ([]auth.ClubMembership, error)
}

func NewService(store *Store, storageService storage.Service, presignExpiryMinutes int, authorizer Authorizer) *Service {
	expiry := time.Duration(presignExpiryMinutes) * time.Minute
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}

	return &Service{
		store:         store,
		storage:       storageService,
		presignExpiry: expiry,
		authorizer:    authorizer,
	}
}

func (s *Service) UploadStudentAvatar(ctx context.Context, studentID string, filename string, contentType string, file io.Reader, declaredSize int64) (*StudentMedia, error) {
	if err := s.requireStudentPermission(ctx, studentID, auth.PermissionMediaManage); err != nil {
		return nil, err
	}
	data, err := readAvatarBytes(file, declaredSize)
	if err != nil {
		return nil, err
	}

	detectedContentType := httpDetectContentType(data)
	if _, ok := avatarMimeTypes[detectedContentType]; !ok {
		return nil, errors.New("avatar must be a JPG, PNG, or WebP image")
	}
	if contentType == "" {
		contentType = detectedContentType
	}

	avatar, err := s.storeAvatarBytes(ctx, studentID, filename, contentType, data, avatarSourceManual)
	if err != nil {
		return nil, err
	}
	if err := s.writeStudentMediaAuditLog(ctx, studentID, avatar.ID, "upload_avatar", nil, avatar, map[string]any{
		"source": avatar.Source,
	}); err != nil {
		return nil, err
	}
	return avatar, nil
}

func (s *Service) ListStudentAvatars(ctx context.Context, studentID string) ([]StudentMedia, error) {
	if err := s.requireAnyStudentPermission(
		ctx,
		studentID,
		auth.PermissionClubRead,
		auth.PermissionStudentsRead,
		auth.PermissionMediaManage,
	); err != nil {
		return nil, err
	}
	exists, err := s.store.StudentExists(ctx, studentID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("student does not exist")
	}

	rows, err := s.store.ListStudentMediaByType(ctx, studentID, mediaTypeAvatar)
	if err != nil {
		return nil, err
	}

	items := make([]StudentMedia, 0, len(rows))
	for _, row := range rows {
		item, err := s.toStudentMedia(ctx, row)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, nil
}

func (s *Service) SetPrimaryAvatar(ctx context.Context, studentID string, mediaID string) (*StudentMedia, error) {
	if err := s.requireStudentPermission(ctx, studentID, auth.PermissionMediaManage); err != nil {
		return nil, err
	}
	beforeRow, err := s.store.GetStudentMediaByID(ctx, studentID, mediaID)
	if err != nil {
		return nil, err
	}
	if beforeRow == nil {
		return nil, errors.New("avatar does not exist")
	}
	if err := s.store.SetPrimaryAvatar(ctx, studentID, mediaID, time.Now().UTC()); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("avatar does not exist")
		}
		return nil, err
	}

	row, err := s.store.GetStudentMediaByID(ctx, studentID, mediaID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errors.New("avatar does not exist")
	}
	avatar, err := s.toStudentMedia(ctx, *row)
	if err != nil {
		return nil, err
	}
	beforeValue := auditStudentMediaValue(*beforeRow)
	if err := s.writeStudentMediaAuditLog(ctx, studentID, mediaID, "set_primary_avatar", beforeValue, avatar, nil); err != nil {
		return nil, err
	}
	return avatar, nil
}

func (s *Service) DeleteAvatar(ctx context.Context, studentID string, mediaID string) error {
	if err := s.requireStudentPermission(ctx, studentID, auth.PermissionMediaManage); err != nil {
		return err
	}
	row, err := s.store.GetStudentMediaByID(ctx, studentID, mediaID)
	if err != nil {
		return err
	}
	if row == nil {
		return errors.New("avatar does not exist")
	}
	oldValue := auditStudentMediaValue(*row)

	if err := s.store.SoftDeleteStudentMedia(ctx, studentID, mediaID, time.Now().UTC()); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("avatar does not exist")
		}
		return err
	}

	if err := s.storage.DeleteObject(ctx, row.StorageKey); err != nil {
		return err
	}
	if row.ThumbnailKey != nil && *row.ThumbnailKey != "" {
		if err := s.storage.DeleteObject(ctx, *row.ThumbnailKey); err != nil {
			return err
		}
	}
	return s.writeStudentMediaAuditLog(ctx, studentID, mediaID, "delete_avatar", oldValue, nil, nil)
}

func (s *Service) requireStudentPermission(ctx context.Context, studentID string, permission string) error {
	return s.requireAnyStudentPermission(ctx, studentID, permission)
}

func (s *Service) requireAnyStudentPermission(ctx context.Context, studentID string, permissionsToCheck ...string) error {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.Subject == "" {
		return errors.New("unauthorized")
	}
	if claims.SystemRole == auth.SystemRoleSysAdmin {
		return nil
	}

	clubID, err := s.store.ResolveStudentClubID(ctx, studentID)
	if err != nil {
		return err
	}
	if clubID == "" {
		return errors.New("student does not exist")
	}

	permissions, err := s.authorizer.GetClubPermissions(ctx, claims.Subject, clubID)
	if err != nil {
		return err
	}
	for _, permission := range permissionsToCheck {
		if permissions.Permissions[permission] {
			return nil
		}
	}

	return errors.New("forbidden")
}

func (s *Service) toStudentMedia(ctx context.Context, row studentMediaRow) (*StudentMedia, error) {
	result := &StudentMedia{
		ID:               row.ID,
		StudentID:        row.StudentID,
		MediaType:        row.MediaType,
		Title:            row.Title,
		Description:      row.Description,
		StorageBucket:    row.StorageBucket,
		StorageKey:       row.StorageKey,
		ThumbnailKey:     row.ThumbnailKey,
		OriginalFilename: row.OriginalFilename,
		MimeType:         row.MimeType,
		FileSize:         row.FileSize,
		ChecksumSHA256:   row.ChecksumSHA256,
		IsPrimary:        row.IsPrimary,
		Source:           row.Source,
		CapturedAt:       formatOptionalTime(row.CapturedAt),
		UploadedAt:       row.UploadedAt.UTC().Format(time.RFC3339Nano),
		CreatedAt:        row.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:        row.UpdatedAt.UTC().Format(time.RFC3339Nano),
		DeletedAt:        formatOptionalTime(row.DeletedAt),
	}

	downloadURL, err := s.storage.PresignDownloadURL(ctx, row.StorageKey, s.presignExpiry)
	if err != nil {
		return nil, err
	}
	result.DownloadURL = downloadURL

	if row.ThumbnailKey != nil && *row.ThumbnailKey != "" {
		thumbnailURL, err := s.storage.PresignDownloadURL(ctx, *row.ThumbnailKey, s.presignExpiry)
		if err != nil {
			return nil, err
		}
		result.ThumbnailURL = thumbnailURL
	}

	return result, nil
}

func (s *Service) writeStudentMediaAuditLog(
	ctx context.Context,
	studentID string,
	mediaID string,
	action string,
	oldValue any,
	newValue any,
	metadata map[string]any,
) error {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.Subject == "" {
		return errors.New("unauthorized")
	}

	clubID, err := s.store.ResolveStudentClubID(ctx, studentID)
	if err != nil {
		return err
	}

	oldValues, err := marshalMediaAuditValue(oldValue)
	if err != nil {
		return err
	}
	newValues, err := marshalMediaAuditValue(newValue)
	if err != nil {
		return err
	}
	metadataValue, err := marshalMediaAuditMetadata(metadata)
	if err != nil {
		return err
	}

	var actorUserID *string
	if claims.Subject != "" {
		actorUserID = &claims.Subject
	}
	var clubIDPtr *string
	if clubID != "" {
		clubIDPtr = &clubID
	}
	var entityID *string
	if mediaID != "" {
		entityID = &mediaID
	}

	return s.store.InsertAuditLog(
		ctx,
		actorUserID,
		clubIDPtr,
		"student_media",
		entityID,
		action,
		oldValues,
		newValues,
		metadataValue,
	)
}

func auditStudentMediaValue(row studentMediaRow) map[string]any {
	return map[string]any{
		"id":               row.ID,
		"studentId":        row.StudentID,
		"mediaType":        row.MediaType,
		"originalFilename": row.OriginalFilename,
		"mimeType":         row.MimeType,
		"fileSize":         row.FileSize,
		"isPrimary":        row.IsPrimary,
		"source":           row.Source,
	}
}

func marshalMediaAuditValue(value any) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func marshalMediaAuditMetadata(metadata map[string]any) (json.RawMessage, error) {
	if metadata == nil {
		return json.RawMessage(`{}`), nil
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func (s *Service) storeAvatarBytes(
	ctx context.Context,
	studentID string,
	filename string,
	contentType string,
	data []byte,
	source string,
) (*StudentMedia, error) {
	if s.storage == nil {
		return nil, errors.New("storage service is not configured")
	}

	exists, err := s.store.StudentExists(ctx, studentID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("student does not exist")
	}

	studentIdentity, err := s.store.GetStudentIdentity(ctx, studentID)
	if err != nil {
		return nil, err
	}
	if studentIdentity == nil {
		return nil, errors.New("student does not exist")
	}

	mediaID, err := generateMediaID()
	if err != nil {
		return nil, err
	}

	readableFilename := buildAvatarReadableFilename(studentIdentity.StudentCode, studentIdentity.FullName, filename, nowTimestamp())
	storageKey := buildAvatarStorageKey(studentID, mediaID, readableFilename)
	thumbnailKey := buildAvatarThumbnailStorageKey(studentID, mediaID)
	now := time.Now().UTC()
	isPrimary := false

	existingAvatarCount, err := s.store.CountStudentMediaByType(ctx, studentID, mediaTypeAvatar)
	if err != nil {
		return nil, err
	}
	if existingAvatarCount == 0 {
		isPrimary = true
	}

	uploadedObject, err := s.storage.UploadObject(ctx, storage.UploadObjectInput{
		Key:          storageKey,
		ContentType:  contentType,
		CacheControl: avatarOriginalCache,
		UserMetadata: map[string]string{
			"x-amz-meta-media-id":      mediaID,
			"x-amz-meta-media-type":    mediaTypeAvatar,
			"x-amz-meta-media-version": mediaID,
			"x-amz-meta-student-id":    studentID,
			"x-amz-meta-object-role":   "original",
		},
		Size: int64(len(data)),
		Body: bytes.NewReader(data),
	})
	if err != nil {
		return nil, err
	}

	thumbnailBytes, err := buildAvatarThumbnail(data)
	if err != nil {
		_ = s.storage.DeleteObject(ctx, uploadedObject.Key)
		return nil, err
	}

	thumbnailObject, err := s.storage.UploadObject(ctx, storage.UploadObjectInput{
		Key:          thumbnailKey,
		ContentType:  "image/jpeg",
		CacheControl: avatarThumbnailCache,
		UserMetadata: map[string]string{
			"x-amz-meta-media-id":      mediaID,
			"x-amz-meta-media-type":    mediaTypeAvatar,
			"x-amz-meta-media-version": mediaID,
			"x-amz-meta-student-id":    studentID,
			"x-amz-meta-object-role":   "thumbnail",
		},
		Size: int64(len(thumbnailBytes)),
		Body: bytes.NewReader(thumbnailBytes),
	})
	if err != nil {
		_ = s.storage.DeleteObject(ctx, uploadedObject.Key)
		return nil, err
	}

	row := studentMediaRow{
		ID:               mediaID,
		StudentID:        studentID,
		MediaType:        mediaTypeAvatar,
		StorageBucket:    uploadedObject.Bucket,
		StorageKey:       uploadedObject.Key,
		ThumbnailKey:     &thumbnailObject.Key,
		OriginalFilename: sanitizeFilename(filename),
		MimeType:         uploadedObject.ContentType,
		FileSize:         uploadedObject.Size,
		IsPrimary:        isPrimary,
		Source:           source,
		UploadedAt:       now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := s.store.InsertStudentMedia(ctx, row); err != nil {
		_ = s.storage.DeleteObject(ctx, uploadedObject.Key)
		_ = s.storage.DeleteObject(ctx, thumbnailObject.Key)
		return nil, err
	}

	return s.toStudentMedia(ctx, row)
}

func buildAvatarStorageKey(studentID string, mediaID string, originalFilename string) string {
	filename := sanitizeFilename(originalFilename)
	if filename == "" {
		filename = "avatar"
	}
	return fmt.Sprintf("students/%s/avatar/%s/%s", studentID, mediaID, filename)
}

func buildAvatarThumbnailStorageKey(studentID string, mediaID string) string {
	return fmt.Sprintf("students/%s/avatar/%s/thumb.jpg", studentID, mediaID)
}

func sanitizeFilename(filename string) string {
	base := strings.TrimSpace(filepath.Base(filename))
	base = strings.ReplaceAll(base, " ", "-")
	base = unsafeFilenameChars.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-.")
	if base == "" {
		return "file"
	}
	return base
}

func buildAvatarReadableFilename(studentCode *string, fullName string, originalFilename string, timestamp string) string {
	extension := strings.ToLower(filepath.Ext(originalFilename))
	if extension == "" {
		extension = ".jpg"
	}

	namePart := normalizeReadableSlug(fullName)
	if namePart == "" {
		namePart = "student"
	}

	prefix := namePart
	if studentCode != nil && strings.TrimSpace(*studentCode) != "" {
		prefix = fmt.Sprintf("%s_%s", strings.TrimSpace(*studentCode), namePart)
	}

	return sanitizeFilename(fmt.Sprintf("%s_%s%s", prefix, timestamp, extension))
}

func normalizeReadableSlug(value string) string {
	normalized := removeAccents(strings.ToLower(strings.TrimSpace(value)))

	var builder strings.Builder
	lastWasDash := false

	for _, r := range normalized {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(r)
			lastWasDash = false
		case unicode.IsSpace(r), r == '-', r == '_':
			if !lastWasDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastWasDash = true
			}
		}
	}

	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return ""
	}

	return unsafeFilenameChars.ReplaceAllString(result, "-")
}

func removeAccents(value string) string {
	transformer := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	normalized, _, err := transform.String(transformer, value)
	if err != nil {
		return value
	}
	return normalized
}

func nowTimestamp() string {
	return time.Now().UTC().Format("20060102T150405Z")
}

func readAvatarBytes(file io.Reader, declaredSize int64) ([]byte, error) {
	if declaredSize > maxAvatarSize {
		return nil, fmt.Errorf("avatar size must be <= %d bytes", maxAvatarSize)
	}

	limited := io.LimitReader(file, maxAvatarSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxAvatarSize {
		return nil, fmt.Errorf("avatar size must be <= %d bytes", maxAvatarSize)
	}
	return data, nil
}

func buildAvatarThumbnail(data []byte) ([]byte, error) {
	source, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, errors.New("failed to decode avatar image")
	}

	thumbnail := imaging.Fill(source, avatarThumbSize, avatarThumbSize, imaging.Center, imaging.Lanczos)

	buffer := bytes.NewBuffer(nil)
	if err := imaging.Encode(buffer, thumbnail, imaging.JPEG, imaging.JPEGQuality(82)); err != nil {
		return nil, errors.New("failed to encode avatar thumbnail")
	}

	return buffer.Bytes(), nil
}

func httpDetectContentType(data []byte) string {
	if len(data) > 512 {
		return httpDetectContentType(data[:512])
	}
	return strings.ToLower(http.DetectContentType(data))
}

func generateMediaID() (string, error) {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return "media_" + strings.ToLower(fmt.Sprintf("%x", buffer)), nil
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339Nano)
	return &formatted
}
