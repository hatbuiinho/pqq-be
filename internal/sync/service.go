package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
)

var studentCodePattern = regexp.MustCompile(`^PQQ-\d{6}$`)
var weekdayPattern = regexp.MustCompile(`^(mon|tue|wed|thu|fri|sat|sun)$`)

type Service struct {
	store Store
	hub   *Hub
}

func NewService(store Store, hub *Hub) *Service {
	return &Service{store: store, hub: hub}
}

func (s *Service) Push(ctx context.Context, request PushRequest) (PushResponse, error) {
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

func (s *Service) Pull(ctx context.Context, request PullRequest) (PullResponse, error) {
	if request.DeviceID == "" {
		return PullResponse{}, errors.New("deviceId is required")
	}
	if request.Limit < 1 {
		request.Limit = 200
	}

	rows, err := s.store.ListChangesSince(ctx, request.Since, request.Limit)
	if err != nil {
		return PullResponse{}, err
	}

	changes := make([]PullChange, 0, len(rows))
	nextSince := request.Since
	for _, row := range rows {
		changes = append(changes, PullChange{
			EntityName:       row.EntityName,
			Record:           row.Payload,
			ServerModifiedAt: row.ServerModifiedAt,
		})
		nextSince = encodeSyncCursor(row.ServerModifiedAt, row.ChangeID)
	}

	return PullResponse{
		ServerTime: time.Now().UTC().Format(time.RFC3339Nano),
		NextSince:  nextSince,
		HasMore:    len(rows) == request.Limit,
		Changes:    changes,
	}, nil
}

func (s *Service) Rebase(ctx context.Context) (RebaseResponse, error) {
	clubs, clubGroups, clubSchedules, beltRanks, students, studentScheduleProfiles, studentSchedules, attendanceSessions, attendanceRecords, err := s.store.ListAllCurrent(ctx)
	if err != nil {
		return RebaseResponse{}, err
	}

	return RebaseResponse{
		ServerTime:              time.Now().UTC().Format(time.RFC3339Nano),
		Clubs:                   clubs,
		ClubGroups:              clubGroups,
		ClubSchedules:           clubSchedules,
		BeltRanks:               beltRanks,
		Students:                students,
		StudentScheduleProfiles: studentScheduleProfiles,
		StudentSchedules:        studentSchedules,
		AttendanceSessions:      attendanceSessions,
		AttendanceRecords:       attendanceRecords,
	}, nil
}

func (s *Service) GetStudentPublicProfile(ctx context.Context, studentCode string) (*StudentPublicProfile, error) {
	return s.store.FindActiveStudentProfileByCode(ctx, studentCode)
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
