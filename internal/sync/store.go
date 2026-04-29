package sync

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

type Store interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	GetRecordForUpdate(ctx context.Context, tx pgx.Tx, entityName EntityName, recordID string) (*StoredRecord, error)
	UpsertRecord(ctx context.Context, tx pgx.Tx, record StoredRecord) error
	UpsertAttendanceRecordsBatch(ctx context.Context, tx pgx.Tx, records []StoredRecord) error
	UpsertStudentMessagesBatch(ctx context.Context, tx pgx.Tx, records []StoredRecord) error
	ListChangesSince(ctx context.Context, since string, limit int) ([]StoredRecord, error)
	IsMutationProcessed(ctx context.Context, tx pgx.Tx, deviceID string, mutationID string) (bool, error)
	SaveProcessedMutation(ctx context.Context, tx pgx.Tx, deviceID string, mutation SyncMutation, serverModifiedAt string) error
	FindClubByCode(ctx context.Context, tx pgx.Tx, clubCode string, excludeID string) (*StoredRecord, error)
	FindClubByName(ctx context.Context, tx pgx.Tx, clubName string) (*StoredRecord, error)
	FindClubGroupByName(ctx context.Context, tx pgx.Tx, clubID string, groupName string, excludeID string) (*StoredRecord, error)
	FindClubScheduleByWeekday(ctx context.Context, tx pgx.Tx, clubID string, weekday string, excludeID string) (*StoredRecord, error)
	FindBeltRankByOrder(ctx context.Context, tx pgx.Tx, order int, excludeID string) (*StoredRecord, error)
	FindBeltRankByName(ctx context.Context, tx pgx.Tx, beltRankName string) (*StoredRecord, error)
	FindStudentByCode(ctx context.Context, tx pgx.Tx, studentCode string, excludeID string) (*StoredRecord, error)
	ResolveUserFullName(ctx context.Context, tx pgx.Tx, userID string) (string, error)
	FindStudentScheduleByWeekday(ctx context.Context, tx pgx.Tx, studentID string, weekday string, excludeID string) (*StoredRecord, error)
	FindAttendanceRecordBySessionAndStudent(ctx context.Context, tx pgx.Tx, sessionID string, studentID string, excludeID string) (*StoredRecord, error)
	ListAttendanceRecordsBySessionForUpdate(ctx context.Context, tx pgx.Tx, sessionID string) ([]StoredRecord, error)
	ListStudentMessagesByAttendanceSessionForUpdate(ctx context.Context, tx pgx.Tx, sessionID string) ([]StoredRecord, error)
	ListClubScheduleWeekdays(ctx context.Context, tx pgx.Tx, clubID string) ([]string, error)
	ReplaceStudentSchedule(ctx context.Context, tx pgx.Tx, studentID string, mode string, weekdays []string, serverNow string) error
	RecordExists(ctx context.Context, tx pgx.Tx, entityName EntityName, recordID string) (bool, error)
	ResolveStudentClubID(ctx context.Context, tx pgx.Tx, studentID string) (string, error)
	ResolveAttendanceSessionClubID(ctx context.Context, tx pgx.Tx, sessionID string) (string, error)
	ResolveStudentClubIDCurrent(ctx context.Context, studentID string) (string, error)
	ResolveAttendanceSessionClubIDCurrent(ctx context.Context, sessionID string) (string, error)
	NextStudentCode(ctx context.Context, tx pgx.Tx) (string, error)
	FindActiveStudentProfileByCode(ctx context.Context, studentCode string) (*StudentPublicProfile, error)
	InsertAuditLog(ctx context.Context, tx pgx.Tx, actorUserID *string, clubID *string, entityType string, entityID *string, action string, oldValues json.RawMessage, newValues json.RawMessage, metadata json.RawMessage) error
	ListAllCurrent(ctx context.Context) ([]ClubRecord, []ClubGroupRecord, []ClubScheduleRecord, []BeltRankRecord, []StudentRecord, []StudentMessageRecord, []StudentScheduleProfileRecord, []StudentScheduleRecord, []AttendanceSessionRecord, []AttendanceRecord, error)
	ListActiveStudentsByClub(ctx context.Context, tx pgx.Tx, clubID string) ([]StudentRecord, error)
	ListActiveClubSchedulesByClub(ctx context.Context, tx pgx.Tx, clubID string) ([]ClubScheduleRecord, error)
	ListActiveStudentScheduleProfilesByStudentIDs(ctx context.Context, tx pgx.Tx, studentIDs []string) ([]StudentScheduleProfileRecord, error)
	ListActiveStudentSchedulesByStudentIDs(ctx context.Context, tx pgx.Tx, studentIDs []string) ([]StudentScheduleRecord, error)
	FindAttendanceSessionByClubAndDate(ctx context.Context, tx pgx.Tx, clubID string, sessionDate string) (*StoredRecord, error)
}
