package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"pqq/be/internal/auth"

	"github.com/jackc/pgx/v5"
)

var studentCodePattern = regexp.MustCompile(`^PQQ-\d{6}$`)
var weekdayPattern = regexp.MustCompile(`^(mon|tue|wed|thu|fri|sat|sun)$`)

type Service struct {
	store      Store
	hub        *Hub
	authorizer MutationAuthorizer
}

type MutationAuthorizer interface {
	GetClubPermissions(ctx context.Context, userID string, clubID string) (*auth.ClubPermissionResponse, error)
	ListMemberships(ctx context.Context, userID string) ([]auth.ClubMembership, error)
}

func NewService(store Store, hub *Hub, authorizer MutationAuthorizer) *Service {
	return &Service{store: store, hub: hub, authorizer: authorizer}
}

func (s *Service) Push(ctx context.Context, request PushRequest) (PushResponse, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.Subject == "" {
		return PushResponse{}, errors.New("unauthorized")
	}
	if request.DeviceID == "" {
		return PushResponse{}, errors.New("deviceId is required")
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return PushResponse{}, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	response := PushResponse{
		ServerTime: now,
		Applied:    make([]AppliedRecord, 0, len(request.Mutations)),
		Conflicts:  make([]Conflict, 0),
	}

	changedEntities := make([]EntityName, 0, len(request.Mutations))
	changedIDs := make([]string, 0, len(request.Mutations))

	for _, mutation := range request.Mutations {
		if err := validateMutation(mutation); err != nil {
			return PushResponse{}, err
		}

		processed, err := s.store.IsMutationProcessed(ctx, tx, request.DeviceID, mutation.MutationID)
		if err != nil {
			return PushResponse{}, err
		}
		if processed {
			continue
		}

		existing, err := s.store.GetRecordForUpdate(ctx, tx, mutation.EntityName, mutation.RecordID)
		if err != nil {
			return PushResponse{}, err
		}

		scope, err := s.authorizeMutation(ctx, tx, claims, mutation, existing)
		if err != nil {
			if conflict, ok := err.(conflictError); ok {
				response.Conflicts = append(response.Conflicts, Conflict{
					MutationID: mutation.MutationID,
					EntityName: mutation.EntityName,
					RecordID:   mutation.RecordID,
					Reason:     conflict.reason,
					Message:    conflict.message,
				})
				continue
			}
			return PushResponse{}, err
		}

		if existing != nil && existing.LastModifiedAt > mutation.ClientModifiedAt {
			response.Conflicts = append(response.Conflicts, Conflict{
				MutationID:   mutation.MutationID,
				EntityName:   mutation.EntityName,
				RecordID:     mutation.RecordID,
				Reason:       "stale_write",
				Message:      "Server record is newer than client mutation.",
				ServerRecord: existing.Payload,
			})
			continue
		}

		canonicalPayload, deletedAt, err := s.canonicalizeMutation(ctx, tx, mutation, now, existing)
		if err != nil {
			if conflict, ok := err.(conflictError); ok {
				response.Conflicts = append(response.Conflicts, Conflict{
					MutationID:   mutation.MutationID,
					EntityName:   mutation.EntityName,
					RecordID:     mutation.RecordID,
					Reason:       conflict.reason,
					Message:      conflict.message,
					ServerRecord: conflict.serverRecord,
				})
				continue
			}
			return PushResponse{}, err
		}

		record := StoredRecord{
			EntityName:       mutation.EntityName,
			RecordID:         mutation.RecordID,
			Payload:          canonicalPayload,
			DeletedAt:        deletedAt,
			LastModifiedAt:   now,
			ServerModifiedAt: now,
		}

		if err := s.store.UpsertRecord(ctx, tx, record); err != nil {
			return PushResponse{}, err
		}
		if err := s.store.SaveProcessedMutation(ctx, tx, request.DeviceID, mutation, now); err != nil {
			return PushResponse{}, err
		}
		if err := s.writeMutationAuditLog(
			ctx,
			tx,
			claims.Subject,
			request.DeviceID,
			scope,
			mutation,
			existing,
			canonicalPayload,
		); err != nil {
			return PushResponse{}, err
		}

		response.Applied = append(response.Applied, AppliedRecord{
			EntityName:       mutation.EntityName,
			Record:           canonicalPayload,
			ServerModifiedAt: now,
		})
		changedEntities = append(changedEntities, mutation.EntityName)
		changedIDs = append(changedIDs, mutation.RecordID)
	}

	if err := tx.Commit(ctx); err != nil {
		return PushResponse{}, err
	}

	if len(changedIDs) > 0 {
		s.hub.BroadcastChange(changedEntities, changedIDs)
	}

	return response, nil
}

func (s *Service) authorizeMutation(
	ctx context.Context,
	tx pgx.Tx,
	claims *auth.Claims,
	mutation SyncMutation,
	existing *StoredRecord,
) (RecordScope, error) {
	scope, err := s.ResolveMutationScope(ctx, tx, mutation, existing)
	if err != nil {
		return RecordScope{}, err
	}

	permission := requiredMutationPermission(mutation)
	if permission == "" {
		return scope, nil
	}

	clubID := scope.ClubID
	permissions, err := s.authorizer.GetClubPermissions(ctx, claims.Subject, clubID)
	if err != nil {
		return RecordScope{}, err
	}
	if permissions.Permissions[permission] {
		return scope, nil
	}

	return RecordScope{}, conflictError{
		reason:  "forbidden",
		message: fmt.Sprintf("You do not have permission to %s for this record.", permission),
	}
}

func (s *Service) writeMutationAuditLog(
	ctx context.Context,
	tx pgx.Tx,
	actorUserID string,
	deviceID string,
	scope RecordScope,
	mutation SyncMutation,
	existing *StoredRecord,
	canonicalPayload []byte,
) error {
	action := mutationAuditAction(mutation, existing)
	oldValues := auditRecordPayload(existing)
	newValues := auditNewValues(mutation.Operation, canonicalPayload)
	metadata, err := json.Marshal(map[string]any{
		"deviceId":         deviceID,
		"mutationId":       mutation.MutationID,
		"operation":        mutation.Operation,
		"clientModifiedAt": mutation.ClientModifiedAt,
	})
	if err != nil {
		return err
	}

	var actorID *string
	if actorUserID != "" {
		actorID = &actorUserID
	}
	var clubID *string
	if !scope.IsGlobal && scope.ClubID != "" {
		clubID = &scope.ClubID
	}
	recordID := mutation.RecordID

	return s.store.InsertAuditLog(
		ctx,
		tx,
		actorID,
		clubID,
		string(mutation.EntityName),
		&recordID,
		action,
		oldValues,
		newValues,
		metadata,
	)
}

func mutationAuditAction(mutation SyncMutation, existing *StoredRecord) string {
	if mutation.Operation == OperationDelete {
		return "delete"
	}
	if existing == nil || existing.DeletedAt != nil {
		return "create"
	}
	return "update"
}

func auditRecordPayload(record *StoredRecord) json.RawMessage {
	if record == nil || len(record.Payload) == 0 {
		return nil
	}
	return json.RawMessage(record.Payload)
}

func auditNewValues(operation Operation, canonicalPayload []byte) json.RawMessage {
	if operation == OperationDelete || len(canonicalPayload) == 0 {
		return nil
	}
	return json.RawMessage(canonicalPayload)
}

func (s *Service) insertImportAuditLog(
	ctx context.Context,
	tx pgx.Tx,
	entityType string,
	action string,
	metadata map[string]any,
) error {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.Subject == "" {
		return errors.New("unauthorized")
	}
	metadataValue, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	actorUserID := claims.Subject
	return s.store.InsertAuditLog(
		ctx,
		tx,
		&actorUserID,
		nil,
		entityType,
		nil,
		action,
		nil,
		nil,
		json.RawMessage(metadataValue),
	)
}

func requiredMutationPermission(mutation SyncMutation) string {
	switch mutation.EntityName {
	case EntityClubs:
		return auth.PermissionClubManage
	case EntityClubGroups:
		return auth.PermissionClubGroupsWrite
	case EntityClubSchedules:
		return auth.PermissionClubManage
	case EntityBeltRanks:
		return auth.PermissionBeltRanksWrite
	case EntityStudents:
		if mutation.Operation == OperationDelete {
			return auth.PermissionStudentsDelete
		}
		return auth.PermissionStudentsWrite
	case EntityStudentMessages:
		return auth.PermissionStudentsWrite
	case EntityStudentScheduleProfiles, EntityStudentSchedules:
		return auth.PermissionStudentsWrite
	case EntityAttendanceSessions, EntityAttendanceRecords:
		return auth.PermissionAttendanceWrite
	default:
		return ""
	}
}

func (s *Service) Pull(ctx context.Context, request PullRequest) (PullResponse, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.Subject == "" {
		return PullResponse{}, errors.New("unauthorized")
	}
	if request.DeviceID == "" {
		return PullResponse{}, errors.New("deviceId is required")
	}
	if request.Limit < 1 {
		request.Limit = 200
	}

	changes := make([]PullChange, 0, request.Limit)
	nextSince := request.Since
	hasMore := false
	permissionCache := make(map[string]map[string]bool)

	for len(changes) < request.Limit {
		rows, err := s.store.ListChangesSince(ctx, nextSince, request.Limit)
		if err != nil {
			return PullResponse{}, err
		}
		if len(rows) == 0 {
			hasMore = false
			break
		}

		for _, row := range rows {
			nextSince = encodeSyncCursor(row.ServerModifiedAt, row.ChangeID)

			allowed, err := s.canReadStoredRecord(ctx, claims, row, permissionCache)
			if err != nil {
				return PullResponse{}, err
			}
			if !allowed {
				continue
			}

			if len(changes) < request.Limit {
				changes = append(changes, PullChange{
					EntityName:       row.EntityName,
					Record:           row.Payload,
					ServerModifiedAt: row.ServerModifiedAt,
				})
			}
		}

		if len(rows) < request.Limit {
			hasMore = false
			break
		}

		if len(changes) >= request.Limit {
			hasMore = true
			break
		}
	}

	return PullResponse{
		ServerTime: time.Now().UTC().Format(time.RFC3339Nano),
		NextSince:  nextSince,
		HasMore:    hasMore,
		Changes:    changes,
	}, nil
}

func (s *Service) Rebase(ctx context.Context) (RebaseResponse, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.Subject == "" {
		return RebaseResponse{}, errors.New("unauthorized")
	}
	clubs, clubGroups, clubSchedules, beltRanks, students, studentMessages, studentScheduleProfiles, studentSchedules, attendanceSessions, attendanceRecords, err := s.store.ListAllCurrent(ctx)
	if err != nil {
		return RebaseResponse{}, err
	}

	allowedClubIDs, err := s.listReadableClubIDs(ctx, claims)
	if err != nil {
		return RebaseResponse{}, err
	}

	if claims.SystemRole != auth.SystemRoleSysAdmin {
		clubs = filterRecords(clubs, func(record ClubRecord) bool {
			return allowedClubIDs[record.ID]
		})
		clubGroups = filterRecords(clubGroups, func(record ClubGroupRecord) bool {
			return allowedClubIDs[record.ClubID]
		})
		clubSchedules = filterRecords(clubSchedules, func(record ClubScheduleRecord) bool {
			return allowedClubIDs[record.ClubID]
		})
		students = filterRecords(students, func(record StudentRecord) bool {
			return allowedClubIDs[record.ClubID]
		})
		studentMessages = filterRecords(studentMessages, func(record StudentMessageRecord) bool {
			return allowedClubIDs[record.ClubID]
		})

		readableStudentIDs := make(map[string]bool, len(students))
		for _, student := range students {
			readableStudentIDs[student.ID] = true
		}

		studentScheduleProfiles = filterRecords(studentScheduleProfiles, func(record StudentScheduleProfileRecord) bool {
			return readableStudentIDs[record.StudentID]
		})
		studentSchedules = filterRecords(studentSchedules, func(record StudentScheduleRecord) bool {
			return readableStudentIDs[record.StudentID]
		})
		attendanceSessions = filterRecords(attendanceSessions, func(record AttendanceSessionRecord) bool {
			return allowedClubIDs[record.ClubID]
		})

		readableSessionIDs := make(map[string]bool, len(attendanceSessions))
		for _, session := range attendanceSessions {
			readableSessionIDs[session.ID] = true
		}

		attendanceRecords = filterRecords(attendanceRecords, func(record AttendanceRecord) bool {
			return readableSessionIDs[record.SessionID] || readableStudentIDs[record.StudentID]
		})
	}

	return RebaseResponse{
		ServerTime:              time.Now().UTC().Format(time.RFC3339Nano),
		Clubs:                   clubs,
		ClubGroups:              clubGroups,
		ClubSchedules:           clubSchedules,
		BeltRanks:               beltRanks,
		Students:                students,
		StudentMessages:         studentMessages,
		StudentScheduleProfiles: studentScheduleProfiles,
		StudentSchedules:        studentSchedules,
		AttendanceSessions:      attendanceSessions,
		AttendanceRecords:       attendanceRecords,
	}, nil
}

func (s *Service) canReadStoredRecord(
	ctx context.Context,
	claims *auth.Claims,
	record StoredRecord,
	permissionCache map[string]map[string]bool,
) (bool, error) {
	scope, err := s.ResolveStoredRecordScope(ctx, record)
	if err != nil {
		return false, err
	}
	if scope.IsGlobal || claims.SystemRole == auth.SystemRoleSysAdmin {
		return true, nil
	}

	permission := requiredReadPermission(record.EntityName)
	if permission == "" {
		return true, nil
	}

	clubPermissions, ok := permissionCache[scope.ClubID]
	if !ok {
		response, err := s.authorizer.GetClubPermissions(ctx, claims.Subject, scope.ClubID)
		if err != nil {
			return false, err
		}
		clubPermissions = response.Permissions
		permissionCache[scope.ClubID] = clubPermissions
	}

	return clubPermissions[permission], nil
}

func (s *Service) listReadableClubIDs(
	ctx context.Context,
	claims *auth.Claims,
) (map[string]bool, error) {
	if claims.SystemRole == auth.SystemRoleSysAdmin {
		return map[string]bool{}, nil
	}

	memberships, err := s.authorizer.ListMemberships(ctx, claims.Subject)
	if err != nil {
		return nil, err
	}

	clubIDs := make(map[string]bool, len(memberships))
	for _, membership := range memberships {
		clubIDs[membership.ClubID] = true
	}
	return clubIDs, nil
}

func requiredReadPermission(entityName EntityName) string {
	switch entityName {
	case EntityClubs:
		return auth.PermissionClubRead
	case EntityClubGroups:
		return auth.PermissionClubGroupsRead
	case EntityClubSchedules:
		return auth.PermissionClubRead
	case EntityBeltRanks:
		return auth.PermissionBeltRanksRead
	case EntityStudents, EntityStudentMessages, EntityStudentScheduleProfiles, EntityStudentSchedules:
		return auth.PermissionStudentsRead
	case EntityAttendanceSessions, EntityAttendanceRecords:
		return auth.PermissionAttendanceRead
	default:
		return ""
	}
}

func filterRecords[T any](items []T, predicate func(item T) bool) []T {
	result := make([]T, 0, len(items))
	for _, item := range items {
		if predicate(item) {
			result = append(result, item)
		}
	}
	return result
}

func (s *Service) GetStudentPublicProfile(ctx context.Context, studentCode string) (*StudentPublicProfile, error) {
	return s.store.FindActiveStudentProfileByCode(ctx, studentCode)
}

func (s *Service) requireSysAdmin(ctx context.Context) error {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.Subject == "" {
		return errors.New("unauthorized")
	}
	if claims.SystemRole != auth.SystemRoleSysAdmin {
		return errors.New("forbidden")
	}
	return nil
}

func (s *Service) canonicalizeMutation(
	ctx context.Context,
	tx pgx.Tx,
	mutation SyncMutation,
	serverNow string,
	existing *StoredRecord,
) ([]byte, *string, error) {
	switch mutation.EntityName {
	case EntityClubs:
		var record ClubRecord
		if err := json.Unmarshal(mutation.Record, &record); err != nil {
			return nil, nil, err
		}
		if record.Name == "" {
			return nil, nil, errors.New("club name is required")
		}
		record.ID = mutation.RecordID
		record.UpdatedAt = serverNow
		record.LastModifiedAt = serverNow
		record.SyncStatus = "synced"
		if record.CreatedAt == "" {
			record.CreatedAt = serverNow
		}
		if mutation.Operation == OperationDelete {
			record.DeletedAt = stringPtr(serverNow)
		}
		payload, err := json.Marshal(record)
		return payload, record.DeletedAt, err
	case EntityClubGroups:
		var record ClubGroupRecord
		if err := json.Unmarshal(mutation.Record, &record); err != nil {
			return nil, nil, err
		}
		if record.Name == "" {
			return nil, nil, errors.New("club group name is required")
		}
		if record.ClubID == "" {
			return nil, nil, errors.New("club group clubId is required")
		}
		clubExists, err := s.store.RecordExists(ctx, tx, EntityClubs, record.ClubID)
		if err != nil {
			return nil, nil, err
		}
		if !clubExists {
			return nil, nil, conflictError{reason: "foreign_key_missing", message: "Club does not exist."}
		}
		duplicated, err := s.store.FindClubGroupByName(ctx, tx, record.ClubID, record.Name, mutation.RecordID)
		if err != nil {
			return nil, nil, err
		}
		if duplicated != nil {
			return nil, nil, conflictError{
				reason:       "duplicate_value",
				message:      "Group name already exists in this club.",
				serverRecord: duplicated.Payload,
			}
		}
		record.ID = mutation.RecordID
		record.UpdatedAt = serverNow
		record.LastModifiedAt = serverNow
		record.SyncStatus = "synced"
		if record.CreatedAt == "" {
			record.CreatedAt = serverNow
		}
		if mutation.Operation == OperationDelete {
			record.DeletedAt = stringPtr(serverNow)
		}
		payload, err := json.Marshal(record)
		return payload, record.DeletedAt, err
	case EntityClubSchedules:
		var record ClubScheduleRecord
		if err := json.Unmarshal(mutation.Record, &record); err != nil {
			return nil, nil, err
		}
		if record.ClubID == "" {
			return nil, nil, errors.New("club schedule clubId is required")
		}
		if !weekdayPattern.MatchString(record.Weekday) {
			return nil, nil, errors.New("club schedule weekday is invalid")
		}
		clubExists, err := s.store.RecordExists(ctx, tx, EntityClubs, record.ClubID)
		if err != nil {
			return nil, nil, err
		}
		if !clubExists {
			return nil, nil, conflictError{reason: "foreign_key_missing", message: "Club does not exist."}
		}
		duplicated, err := s.store.FindClubScheduleByWeekday(ctx, tx, record.ClubID, record.Weekday, mutation.RecordID)
		if err != nil {
			return nil, nil, err
		}
		if duplicated != nil {
			return nil, nil, conflictError{
				reason:       "duplicate_value",
				message:      "Club training day already exists.",
				serverRecord: duplicated.Payload,
			}
		}
		record.ID = mutation.RecordID
		record.UpdatedAt = serverNow
		record.LastModifiedAt = serverNow
		record.SyncStatus = "synced"
		if record.CreatedAt == "" {
			record.CreatedAt = serverNow
		}
		if mutation.Operation == OperationDelete {
			record.DeletedAt = stringPtr(serverNow)
		}
		payload, err := json.Marshal(record)
		return payload, record.DeletedAt, err
	case EntityBeltRanks:
		var record BeltRankRecord
		if err := json.Unmarshal(mutation.Record, &record); err != nil {
			return nil, nil, err
		}
		if record.Name == "" {
			return nil, nil, errors.New("belt rank name is required")
		}
		if record.Order < 1 {
			return nil, nil, errors.New("belt rank order must be >= 1")
		}
		duplicated, err := s.store.FindBeltRankByOrder(ctx, tx, record.Order, mutation.RecordID)
		if err != nil {
			return nil, nil, err
		}
		if duplicated != nil {
			return nil, nil, conflictError{
				reason:       "duplicate_value",
				message:      "Belt rank order already exists.",
				serverRecord: duplicated.Payload,
			}
		}
		record.ID = mutation.RecordID
		record.UpdatedAt = serverNow
		record.LastModifiedAt = serverNow
		record.SyncStatus = "synced"
		if record.CreatedAt == "" {
			record.CreatedAt = serverNow
		}
		if mutation.Operation == OperationDelete {
			record.DeletedAt = stringPtr(serverNow)
		}
		payload, err := json.Marshal(record)
		return payload, record.DeletedAt, err
	case EntityStudents:
		var record StudentRecord
		if err := json.Unmarshal(mutation.Record, &record); err != nil {
			return nil, nil, err
		}
		if record.FullName == "" {
			return nil, nil, errors.New("student fullName is required")
		}
		if record.ClubID == "" || record.BeltRankID == "" {
			return nil, nil, errors.New("student clubId and beltRankId are required")
		}

		clubExists, err := s.store.RecordExists(ctx, tx, EntityClubs, record.ClubID)
		if err != nil {
			return nil, nil, err
		}
		if !clubExists {
			return nil, nil, conflictError{reason: "foreign_key_missing", message: "Club does not exist."}
		}

		if record.GroupID != nil && *record.GroupID != "" {
			groupExists, err := s.store.RecordExists(ctx, tx, EntityClubGroups, *record.GroupID)
			if err != nil {
				return nil, nil, err
			}
			if !groupExists {
				return nil, nil, conflictError{reason: "foreign_key_missing", message: "Group does not exist."}
			}
		}

		beltRankExists, err := s.store.RecordExists(ctx, tx, EntityBeltRanks, record.BeltRankID)
		if err != nil {
			return nil, nil, err
		}
		if !beltRankExists {
			return nil, nil, conflictError{reason: "foreign_key_missing", message: "Belt rank does not exist."}
		}

		if record.StudentCode == nil || *record.StudentCode == "" {
			code, err := s.store.NextStudentCode(ctx, tx)
			if err != nil {
				return nil, nil, err
			}
			record.StudentCode = &code
		} else if !studentCodePattern.MatchString(*record.StudentCode) {
			return nil, nil, conflictError{reason: "validation_failed", message: "Student code format is invalid."}
		}

		duplicated, err := s.store.FindStudentByCode(ctx, tx, *record.StudentCode, mutation.RecordID)
		if err != nil {
			return nil, nil, err
		}
		if duplicated != nil {
			return nil, nil, conflictError{
				reason:       "duplicate_value",
				message:      "Student code already exists.",
				serverRecord: duplicated.Payload,
			}
		}

		record.ID = mutation.RecordID
		record.UpdatedAt = serverNow
		record.LastModifiedAt = serverNow
		record.SyncStatus = "synced"
		if record.CreatedAt == "" {
			record.CreatedAt = serverNow
		}
		if mutation.Operation == OperationDelete {
			record.DeletedAt = stringPtr(serverNow)
		}
		payload, err := json.Marshal(record)
		return payload, record.DeletedAt, err
	case EntityStudentMessages:
		var record StudentMessageRecord
		if err := json.Unmarshal(mutation.Record, &record); err != nil {
			return nil, nil, err
		}
		if record.StudentID == "" || record.ClubID == "" {
			return nil, nil, errors.New("student message studentId and clubId are required")
		}
		if record.MessageType != "manual" && record.MessageType != "attendance_note" {
			return nil, nil, errors.New("student message type is invalid")
		}
		if mutation.Operation != OperationDelete && record.MessageType == "attendance_note" {
			return nil, nil, conflictError{
				reason:  "validation_failed",
				message: "Attendance note messages are managed by the attendance system.",
			}
		}
		if mutation.Operation != OperationDelete && strings.TrimSpace(record.AuthorName) == "" {
			return nil, nil, errors.New("student message authorName is required")
		}
		if mutation.Operation != OperationDelete && record.MessageType == "manual" && (record.AuthorUserID == nil || *record.AuthorUserID == "") {
			return nil, nil, errors.New("student message authorUserId is required")
		}
		if strings.TrimSpace(record.Content) == "" && mutation.Operation != OperationDelete {
			return nil, nil, errors.New("student message content is required")
		}
		studentExists, err := s.store.RecordExists(ctx, tx, EntityStudents, record.StudentID)
		if err != nil {
			return nil, nil, err
		}
		if !studentExists {
			return nil, nil, conflictError{reason: "foreign_key_missing", message: "Student does not exist."}
		}
		clubExists, err := s.store.RecordExists(ctx, tx, EntityClubs, record.ClubID)
		if err != nil {
			return nil, nil, err
		}
		if !clubExists {
			return nil, nil, conflictError{reason: "foreign_key_missing", message: "Club does not exist."}
		}
		record.ID = mutation.RecordID
		record.Content = strings.TrimSpace(record.Content)
		record.UpdatedAt = serverNow
		record.LastModifiedAt = serverNow
		record.SyncStatus = "synced"
		if record.CreatedAt == "" {
			record.CreatedAt = serverNow
		}
		if mutation.Operation == OperationDelete {
			record.DeletedAt = stringPtr(serverNow)
		}
		payload, err := json.Marshal(record)
		return payload, record.DeletedAt, err
	case EntityStudentScheduleProfiles:
		var record StudentScheduleProfileRecord
		if err := json.Unmarshal(mutation.Record, &record); err != nil {
			return nil, nil, err
		}
		if record.StudentID == "" {
			return nil, nil, errors.New("student schedule profile studentId is required")
		}
		if record.Mode != "inherit" && record.Mode != "custom" {
			return nil, nil, errors.New("student schedule profile mode is invalid")
		}
		studentExists, err := s.store.RecordExists(ctx, tx, EntityStudents, record.StudentID)
		if err != nil {
			return nil, nil, err
		}
		if !studentExists {
			return nil, nil, conflictError{reason: "foreign_key_missing", message: "Student does not exist."}
		}
		record.ID = mutation.RecordID
		record.UpdatedAt = serverNow
		record.LastModifiedAt = serverNow
		record.SyncStatus = "synced"
		if record.CreatedAt == "" {
			record.CreatedAt = serverNow
		}
		if mutation.Operation == OperationDelete {
			record.DeletedAt = stringPtr(serverNow)
		}
		payload, err := json.Marshal(record)
		return payload, record.DeletedAt, err
	case EntityStudentSchedules:
		var record StudentScheduleRecord
		if err := json.Unmarshal(mutation.Record, &record); err != nil {
			return nil, nil, err
		}
		if record.StudentID == "" {
			return nil, nil, errors.New("student schedule studentId is required")
		}
		if !weekdayPattern.MatchString(record.Weekday) {
			return nil, nil, errors.New("student schedule weekday is invalid")
		}
		studentExists, err := s.store.RecordExists(ctx, tx, EntityStudents, record.StudentID)
		if err != nil {
			return nil, nil, err
		}
		if !studentExists {
			return nil, nil, conflictError{reason: "foreign_key_missing", message: "Student does not exist."}
		}
		duplicated, err := s.store.FindStudentScheduleByWeekday(ctx, tx, record.StudentID, record.Weekday, mutation.RecordID)
		if err != nil {
			return nil, nil, err
		}
		if duplicated != nil {
			return nil, nil, conflictError{
				reason:       "duplicate_value",
				message:      "Student schedule day already exists.",
				serverRecord: duplicated.Payload,
			}
		}
		record.ID = mutation.RecordID
		record.UpdatedAt = serverNow
		record.LastModifiedAt = serverNow
		record.SyncStatus = "synced"
		if record.CreatedAt == "" {
			record.CreatedAt = serverNow
		}
		if mutation.Operation == OperationDelete {
			record.DeletedAt = stringPtr(serverNow)
		}
		payload, err := json.Marshal(record)
		return payload, record.DeletedAt, err
	case EntityAttendanceSessions:
		var record AttendanceSessionRecord
		if err := json.Unmarshal(mutation.Record, &record); err != nil {
			return nil, nil, err
		}
		if record.ClubID == "" {
			return nil, nil, errors.New("attendance session clubId is required")
		}
		if record.SessionDate == "" {
			return nil, nil, errors.New("attendance session sessionDate is required")
		}
		if record.Status != "draft" && record.Status != "completed" {
			return nil, nil, errors.New("attendance session status is invalid")
		}
		if mutation.Operation != OperationDelete && record.Status == "draft" && existing != nil {
			var existingRecord AttendanceSessionRecord
			if err := json.Unmarshal(existing.Payload, &existingRecord); err == nil {
				if existingRecord.Status == "completed" && existingRecord.SessionDate != rfc3339Date(serverNow) {
					return nil, nil, conflictError{
						reason:  "business_rule_violation",
						message: "Only sessions scheduled for today can be reopened.",
					}
				}
			}
		}
		clubExists, err := s.store.RecordExists(ctx, tx, EntityClubs, record.ClubID)
		if err != nil {
			return nil, nil, err
		}
		if !clubExists {
			return nil, nil, conflictError{reason: "foreign_key_missing", message: "Club does not exist."}
		}
		record.ID = mutation.RecordID
		record.UpdatedAt = serverNow
		record.LastModifiedAt = serverNow
		record.SyncStatus = "synced"
		if record.CreatedAt == "" {
			record.CreatedAt = serverNow
		}
		if mutation.Operation == OperationDelete {
			record.DeletedAt = stringPtr(serverNow)
		}
		payload, err := json.Marshal(record)
		return payload, record.DeletedAt, err
	case EntityAttendanceRecords:
		var record AttendanceRecord
		if err := json.Unmarshal(mutation.Record, &record); err != nil {
			return nil, nil, err
		}
		if record.SessionID == "" {
			return nil, nil, errors.New("attendance record sessionId is required")
		}
		if record.StudentID == "" {
			return nil, nil, errors.New("attendance record studentId is required")
		}
		switch record.AttendanceStatus {
		case "unmarked", "present", "late", "excused", "left_early", "absent":
		default:
			return nil, nil, errors.New("attendance record status is invalid")
		}
		if mutation.Operation != OperationDelete {
			sessionExists, err := s.store.RecordExists(ctx, tx, EntityAttendanceSessions, record.SessionID)
			if err != nil {
				return nil, nil, err
			}
			if !sessionExists {
				return nil, nil, conflictError{reason: "foreign_key_missing", message: "Attendance session does not exist."}
			}
			studentExists, err := s.store.RecordExists(ctx, tx, EntityStudents, record.StudentID)
			if err != nil {
				return nil, nil, err
			}
			if !studentExists {
				return nil, nil, conflictError{reason: "foreign_key_missing", message: "Student does not exist."}
			}
			duplicated, err := s.store.FindAttendanceRecordBySessionAndStudent(ctx, tx, record.SessionID, record.StudentID, mutation.RecordID)
			if err != nil {
				return nil, nil, err
			}
			if duplicated != nil {
				return nil, nil, conflictError{
					reason:       "duplicate_value",
					message:      "Attendance record already exists for this student in the session.",
					serverRecord: duplicated.Payload,
				}
			}
		}
		record.ID = mutation.RecordID
		record.UpdatedAt = serverNow
		record.LastModifiedAt = serverNow
		record.SyncStatus = "synced"
		if record.CreatedAt == "" {
			record.CreatedAt = serverNow
		}
		if mutation.Operation == OperationDelete {
			record.DeletedAt = stringPtr(serverNow)
		}
		payload, err := json.Marshal(record)
		return payload, record.DeletedAt, err
	default:
		return nil, nil, fmt.Errorf("unsupported entityName %q", mutation.EntityName)
	}
}

func validateMutation(mutation SyncMutation) error {
	if mutation.MutationID == "" {
		return errors.New("mutationId is required")
	}
	if mutation.RecordID == "" {
		return errors.New("recordId is required")
	}
	if mutation.ClientModifiedAt == "" {
		return errors.New("clientModifiedAt is required")
	}
	if _, err := time.Parse(time.RFC3339Nano, mutation.ClientModifiedAt); err != nil {
		return errors.New("clientModifiedAt must be RFC3339 timestamp")
	}
	if mutation.Operation != OperationUpsert && mutation.Operation != OperationDelete {
		return errors.New("operation must be upsert or delete")
	}
	return nil
}

type conflictError struct {
	reason       string
	message      string
	serverRecord []byte
}

func (e conflictError) Error() string {
	return e.message
}

func stringPtr(value string) *string {
	return &value
}

func rfc3339Date(value string) string {
	if len(value) >= 10 {
		return value[:10]
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return ""
	}
	return parsed.UTC().Format("2006-01-02")
}

func encodeSyncCursor(serverModifiedAt string, changeID int64) string {
	if serverModifiedAt == "" {
		return ""
	}
	return fmt.Sprintf("%s#%d", serverModifiedAt, changeID)
}
