package sync

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

type RecordScope struct {
	ClubID   string
	IsGlobal bool
}

func (s *Service) ResolveMutationScope(
	ctx context.Context,
	tx pgx.Tx,
	mutation SyncMutation,
	existing *StoredRecord,
) (RecordScope, error) {
	switch mutation.EntityName {
	case EntityBeltRanks:
		return RecordScope{IsGlobal: true}, nil
	case EntityClubs:
		return RecordScope{ClubID: mutation.RecordID}, nil
	case EntityClubGroups:
		return resolveClubLinkedScope[ClubGroupRecord](ctx, tx, s.store, mutation, existing, func(record ClubGroupRecord) string {
			return record.ClubID
		})
	case EntityClubSchedules:
		return resolveClubLinkedScope[ClubScheduleRecord](ctx, tx, s.store, mutation, existing, func(record ClubScheduleRecord) string {
			return record.ClubID
		})
	case EntityStudents:
		return resolveClubLinkedScope[StudentRecord](ctx, tx, s.store, mutation, existing, func(record StudentRecord) string {
			return record.ClubID
		})
	case EntityStudentScheduleProfiles:
		return resolveStudentLinkedScope[StudentScheduleProfileRecord](ctx, tx, s.store, mutation, existing, func(record StudentScheduleProfileRecord) string {
			return record.StudentID
		})
	case EntityStudentSchedules:
		return resolveStudentLinkedScope[StudentScheduleRecord](ctx, tx, s.store, mutation, existing, func(record StudentScheduleRecord) string {
			return record.StudentID
		})
	case EntityAttendanceSessions:
		return resolveClubLinkedScope[AttendanceSessionRecord](ctx, tx, s.store, mutation, existing, func(record AttendanceSessionRecord) string {
			return record.ClubID
		})
	case EntityAttendanceRecords:
		return resolveAttendanceRecordScope(ctx, tx, s.store, mutation, existing)
	default:
		return RecordScope{}, errors.New("unsupported entity for authorization scope")
	}
}

func (s *Service) ResolveStoredRecordScope(ctx context.Context, record StoredRecord) (RecordScope, error) {
	switch record.EntityName {
	case EntityBeltRanks:
		return RecordScope{IsGlobal: true}, nil
	case EntityClubs:
		return RecordScope{ClubID: record.RecordID}, nil
	case EntityClubGroups:
		return resolveStoredClubLinkedScope[ClubGroupRecord](record, func(value ClubGroupRecord) string {
			return value.ClubID
		})
	case EntityClubSchedules:
		return resolveStoredClubLinkedScope[ClubScheduleRecord](record, func(value ClubScheduleRecord) string {
			return value.ClubID
		})
	case EntityStudents:
		return resolveStoredClubLinkedScope[StudentRecord](record, func(value StudentRecord) string {
			return value.ClubID
		})
	case EntityStudentScheduleProfiles:
		return resolveStoredStudentLinkedScope[StudentScheduleProfileRecord](ctx, s.store, record, func(value StudentScheduleProfileRecord) string {
			return value.StudentID
		})
	case EntityStudentSchedules:
		return resolveStoredStudentLinkedScope[StudentScheduleRecord](ctx, s.store, record, func(value StudentScheduleRecord) string {
			return value.StudentID
		})
	case EntityAttendanceSessions:
		return resolveStoredClubLinkedScope[AttendanceSessionRecord](record, func(value AttendanceSessionRecord) string {
			return value.ClubID
		})
	case EntityAttendanceRecords:
		return resolveStoredAttendanceRecordScope(ctx, s.store, record)
	default:
		return RecordScope{}, errors.New("unsupported entity for authorization scope")
	}
}

func resolveClubLinkedScope[T any](
	ctx context.Context,
	tx pgx.Tx,
	store Store,
	mutation SyncMutation,
	existing *StoredRecord,
	resolveClubID func(record T) string,
) (RecordScope, error) {
	record, err := decodeMutationRecord[T](mutation, existing)
	if err != nil {
		return RecordScope{}, err
	}

	clubID := resolveClubID(record)
	if clubID == "" {
		return RecordScope{}, errors.New("club scope cannot be resolved")
	}

	return RecordScope{ClubID: clubID}, nil
}

func resolveStudentLinkedScope[T any](
	ctx context.Context,
	tx pgx.Tx,
	store Store,
	mutation SyncMutation,
	existing *StoredRecord,
	resolveStudentID func(record T) string,
) (RecordScope, error) {
	record, err := decodeMutationRecord[T](mutation, existing)
	if err != nil {
		return RecordScope{}, err
	}

	studentID := resolveStudentID(record)
	if studentID == "" {
		return RecordScope{}, errors.New("student scope cannot be resolved")
	}

	clubID, err := store.ResolveStudentClubID(ctx, tx, studentID)
	if err != nil {
		return RecordScope{}, err
	}
	if clubID == "" {
		return RecordScope{}, errors.New("student club scope cannot be resolved")
	}

	return RecordScope{ClubID: clubID}, nil
}

func resolveAttendanceRecordScope(
	ctx context.Context,
	tx pgx.Tx,
	store Store,
	mutation SyncMutation,
	existing *StoredRecord,
) (RecordScope, error) {
	record, err := decodeMutationRecord[AttendanceRecord](mutation, existing)
	if err != nil {
		return RecordScope{}, err
	}

	if record.SessionID != "" {
		clubID, err := store.ResolveAttendanceSessionClubID(ctx, tx, record.SessionID)
		if err != nil {
			return RecordScope{}, err
		}
		if clubID != "" {
			return RecordScope{ClubID: clubID}, nil
		}
	}

	if record.StudentID != "" {
		clubID, err := store.ResolveStudentClubID(ctx, tx, record.StudentID)
		if err != nil {
			return RecordScope{}, err
		}
		if clubID != "" {
			return RecordScope{ClubID: clubID}, nil
		}
	}

	return RecordScope{}, errors.New("attendance record club scope cannot be resolved")
}

func decodeMutationRecord[T any](mutation SyncMutation, existing *StoredRecord) (T, error) {
	var record T
	if len(mutation.Record) > 0 {
		if err := json.Unmarshal(mutation.Record, &record); err != nil {
			return record, err
		}
		return record, nil
	}
	if existing == nil {
		return record, errors.New("record scope cannot be resolved")
	}
	if err := json.Unmarshal(existing.Payload, &record); err != nil {
		return record, err
	}
	return record, nil
}

func resolveStoredClubLinkedScope[T any](
	record StoredRecord,
	resolveClubID func(record T) string,
) (RecordScope, error) {
	value, err := decodeStoredRecord[T](record)
	if err != nil {
		return RecordScope{}, err
	}

	clubID := resolveClubID(value)
	if clubID == "" {
		return RecordScope{}, errors.New("club scope cannot be resolved")
	}

	return RecordScope{ClubID: clubID}, nil
}

func resolveStoredStudentLinkedScope[T any](
	ctx context.Context,
	store Store,
	record StoredRecord,
	resolveStudentID func(record T) string,
) (RecordScope, error) {
	value, err := decodeStoredRecord[T](record)
	if err != nil {
		return RecordScope{}, err
	}

	studentID := resolveStudentID(value)
	if studentID == "" {
		return RecordScope{}, errors.New("student scope cannot be resolved")
	}

	clubID, err := store.ResolveStudentClubIDCurrent(ctx, studentID)
	if err != nil {
		return RecordScope{}, err
	}
	if clubID == "" {
		return RecordScope{}, errors.New("student club scope cannot be resolved")
	}

	return RecordScope{ClubID: clubID}, nil
}

func resolveStoredAttendanceRecordScope(
	ctx context.Context,
	store Store,
	record StoredRecord,
) (RecordScope, error) {
	value, err := decodeStoredRecord[AttendanceRecord](record)
	if err != nil {
		return RecordScope{}, err
	}

	if value.SessionID != "" {
		clubID, err := store.ResolveAttendanceSessionClubIDCurrent(ctx, value.SessionID)
		if err != nil {
			return RecordScope{}, err
		}
		if clubID != "" {
			return RecordScope{ClubID: clubID}, nil
		}
	}

	if value.StudentID != "" {
		clubID, err := store.ResolveStudentClubIDCurrent(ctx, value.StudentID)
		if err != nil {
			return RecordScope{}, err
		}
		if clubID != "" {
			return RecordScope{ClubID: clubID}, nil
		}
	}

	return RecordScope{}, errors.New("attendance record club scope cannot be resolved")
}

func decodeStoredRecord[T any](record StoredRecord) (T, error) {
	var value T
	if err := json.Unmarshal(record.Payload, &value); err != nil {
		return value, err
	}
	return value, nil
}
