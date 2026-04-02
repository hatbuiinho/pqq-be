package sync

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type Store interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	GetRecordForUpdate(ctx context.Context, tx pgx.Tx, entityName EntityName, recordID string) (*StoredRecord, error)
	UpsertRecord(ctx context.Context, tx pgx.Tx, record StoredRecord) error
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
	FindStudentScheduleByWeekday(ctx context.Context, tx pgx.Tx, studentID string, weekday string, excludeID string) (*StoredRecord, error)
	FindAttendanceRecordBySessionAndStudent(ctx context.Context, tx pgx.Tx, sessionID string, studentID string, excludeID string) (*StoredRecord, error)
	ListClubScheduleWeekdays(ctx context.Context, tx pgx.Tx, clubID string) ([]string, error)
	ReplaceStudentSchedule(ctx context.Context, tx pgx.Tx, studentID string, mode string, weekdays []string, serverNow string) error
	RecordExists(ctx context.Context, tx pgx.Tx, entityName EntityName, recordID string) (bool, error)
	NextStudentCode(ctx context.Context, tx pgx.Tx) (string, error)
	FindActiveStudentProfileByCode(ctx context.Context, studentCode string) (*StudentPublicProfile, error)
	ListAllCurrent(ctx context.Context) ([]ClubRecord, []ClubGroupRecord, []ClubScheduleRecord, []BeltRankRecord, []StudentRecord, []StudentScheduleProfileRecord, []StudentScheduleRecord, []AttendanceSessionRecord, []AttendanceRecord, error)
}
