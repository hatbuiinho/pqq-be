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
	FindBeltRankByOrder(ctx context.Context, tx pgx.Tx, order int, excludeID string) (*StoredRecord, error)
	FindStudentByCode(ctx context.Context, tx pgx.Tx, studentCode string, excludeID string) (*StoredRecord, error)
	RecordExists(ctx context.Context, tx pgx.Tx, entityName EntityName, recordID string) (bool, error)
	NextStudentCode(ctx context.Context, tx pgx.Tx) (string, error)
	ListAllCurrent(ctx context.Context) ([]ClubRecord, []BeltRankRecord, []StudentRecord, error)
}
