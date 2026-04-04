package media

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

type studentMediaRow struct {
	ID               string
	StudentID        string
	MediaType        string
	Title            *string
	Description      *string
	StorageBucket    string
	StorageKey       string
	ThumbnailKey     *string
	OriginalFilename string
	MimeType         string
	FileSize         int64
	ChecksumSHA256   *string
	IsPrimary        bool
	Source           string
	CapturedAt       *time.Time
	UploadedAt       time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

type studentIdentity struct {
	ClubID      string
	StudentCode *string
	FullName    string
}

type studentLookupRow struct {
	ID          string
	ClubID      string
	StudentCode *string
	FullName    string
}

type mediaImportBatchRow struct {
	ID             string
	Status         string
	SourceType     string
	OriginalFile   *string
	TotalItems     int
	MatchedItems   int
	AmbiguousItems int
	UnmatchedItems int
	FailedItems    int
	ImportedItems  int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ProcessedAt    *time.Time
	DeletedAt      *time.Time
}

type mediaImportBatchItemRow struct {
	ID                 string
	BatchID            string
	OriginalFilename   string
	TempStorageBucket  string
	TempStorageKey     string
	MimeType           string
	FileSize           int64
	GuessedStudentID   *string
	GuessedStudentName *string
	MatchMethod        *string
	MatchScore         *int
	ConfirmedStudentID *string
	MediaType          string
	Title              *string
	Description        *string
	Status             string
	ErrorMessage       *string
	FinalMediaID       *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}

func (s *Store) StudentExists(ctx context.Context, studentID string) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM students WHERE id = $1 AND deleted_at IS NULL)`, studentID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Store) GetStudentIdentity(ctx context.Context, studentID string) (*studentIdentity, error) {
	query := `
		SELECT club_id, student_code, full_name
		FROM students
		WHERE id = $1
			AND deleted_at IS NULL
		LIMIT 1
	`

	var identity studentIdentity
	if err := s.pool.QueryRow(ctx, query, studentID).Scan(&identity.ClubID, &identity.StudentCode, &identity.FullName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &identity, nil
}

func (s *Store) ListActiveStudentsForImport(ctx context.Context) ([]studentLookupRow, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, club_id, student_code, full_name FROM students WHERE deleted_at IS NULL ORDER BY full_name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]studentLookupRow, 0)
	for rows.Next() {
		var row studentLookupRow
		if err := rows.Scan(&row.ID, &row.ClubID, &row.StudentCode, &row.FullName); err != nil {
			return nil, err
		}
		result = append(result, row)
	}

	return result, rows.Err()
}

func (s *Store) ResolveStudentClubID(ctx context.Context, studentID string) (string, error) {
	var clubID string
	if err := s.pool.QueryRow(
		ctx,
		`SELECT club_id FROM students WHERE id = $1 AND deleted_at IS NULL LIMIT 1`,
		studentID,
	).Scan(&clubID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return clubID, nil
}

func (s *Store) InsertStudentMedia(ctx context.Context, row studentMediaRow) error {
	query := `
		INSERT INTO student_media (
			id,
			student_id,
			media_type,
			title,
			description,
			storage_bucket,
			storage_key,
			thumbnail_key,
			original_filename,
			mime_type,
			file_size,
			checksum_sha256,
			is_primary,
			source,
			captured_at,
			uploaded_at,
			created_at,
			updated_at,
			deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19
		)
	`

	_, err := s.pool.Exec(
		ctx,
		query,
		row.ID,
		row.StudentID,
		row.MediaType,
		row.Title,
		row.Description,
		row.StorageBucket,
		row.StorageKey,
		row.ThumbnailKey,
		row.OriginalFilename,
		row.MimeType,
		row.FileSize,
		row.ChecksumSHA256,
		row.IsPrimary,
		row.Source,
		row.CapturedAt,
		row.UploadedAt,
		row.CreatedAt,
		row.UpdatedAt,
		row.DeletedAt,
	)
	return err
}

func (s *Store) ListStudentMediaByType(ctx context.Context, studentID string, mediaType string) ([]studentMediaRow, error) {
	query := `
		SELECT id, student_id, media_type, title, description, storage_bucket, storage_key, thumbnail_key,
			original_filename, mime_type, file_size, checksum_sha256, is_primary, source, captured_at,
			uploaded_at, created_at, updated_at, deleted_at
		FROM student_media
		WHERE student_id = $1
			AND media_type = $2
			AND deleted_at IS NULL
		ORDER BY is_primary DESC, uploaded_at DESC
	`

	rows, err := s.pool.Query(ctx, query, studentID, mediaType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]studentMediaRow, 0)
	for rows.Next() {
		var row studentMediaRow
		if err := rows.Scan(
			&row.ID,
			&row.StudentID,
			&row.MediaType,
			&row.Title,
			&row.Description,
			&row.StorageBucket,
			&row.StorageKey,
			&row.ThumbnailKey,
			&row.OriginalFilename,
			&row.MimeType,
			&row.FileSize,
			&row.ChecksumSHA256,
			&row.IsPrimary,
			&row.Source,
			&row.CapturedAt,
			&row.UploadedAt,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.DeletedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}

	return result, rows.Err()
}

func (s *Store) GetStudentMediaByID(ctx context.Context, studentID string, mediaID string) (*studentMediaRow, error) {
	query := `
		SELECT id, student_id, media_type, title, description, storage_bucket, storage_key, thumbnail_key,
			original_filename, mime_type, file_size, checksum_sha256, is_primary, source, captured_at,
			uploaded_at, created_at, updated_at, deleted_at
		FROM student_media
		WHERE id = $1
			AND student_id = $2
			AND deleted_at IS NULL
		LIMIT 1
	`

	var row studentMediaRow
	err := s.pool.QueryRow(ctx, query, mediaID, studentID).Scan(
		&row.ID,
		&row.StudentID,
		&row.MediaType,
		&row.Title,
		&row.Description,
		&row.StorageBucket,
		&row.StorageKey,
		&row.ThumbnailKey,
		&row.OriginalFilename,
		&row.MimeType,
		&row.FileSize,
		&row.ChecksumSHA256,
		&row.IsPrimary,
		&row.Source,
		&row.CapturedAt,
		&row.UploadedAt,
		&row.CreatedAt,
		&row.UpdatedAt,
		&row.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &row, nil
}

func (s *Store) CountStudentMediaByType(ctx context.Context, studentID string, mediaType string) (int, error) {
	var count int
	if err := s.pool.QueryRow(
		ctx,
		`SELECT COUNT(1) FROM student_media WHERE student_id = $1 AND media_type = $2 AND deleted_at IS NULL`,
		studentID,
		mediaType,
	).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) SetPrimaryAvatar(ctx context.Context, studentID string, mediaID string, updatedAt time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(
		ctx,
		`UPDATE student_media SET is_primary = FALSE, updated_at = $2 WHERE student_id = $1 AND media_type = 'avatar' AND deleted_at IS NULL`,
		studentID,
		updatedAt,
	); err != nil {
		return err
	}

	commandTag, err := tx.Exec(
		ctx,
		`UPDATE student_media SET is_primary = TRUE, updated_at = $3 WHERE id = $1 AND student_id = $2 AND media_type = 'avatar' AND deleted_at IS NULL`,
		mediaID,
		studentID,
		updatedAt,
	)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return tx.Commit(ctx)
}

func (s *Store) SoftDeleteStudentMedia(ctx context.Context, studentID string, mediaID string, deletedAt time.Time) error {
	commandTag, err := s.pool.Exec(
		ctx,
		`UPDATE student_media SET deleted_at = $3, updated_at = $3, is_primary = FALSE WHERE id = $1 AND student_id = $2 AND deleted_at IS NULL`,
		mediaID,
		studentID,
		deletedAt,
	)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) InsertMediaImportBatch(ctx context.Context, row mediaImportBatchRow) error {
	_, err := s.pool.Exec(
		ctx,
		`INSERT INTO media_import_batches (
			id, status, source_type, original_filename, total_items, matched_items, ambiguous_items,
			unmatched_items, failed_items, imported_items, created_at, updated_at, processed_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)`,
		row.ID,
		row.Status,
		row.SourceType,
		row.OriginalFile,
		row.TotalItems,
		row.MatchedItems,
		row.AmbiguousItems,
		row.UnmatchedItems,
		row.FailedItems,
		row.ImportedItems,
		row.CreatedAt,
		row.UpdatedAt,
		row.ProcessedAt,
		row.DeletedAt,
	)
	return err
}

func (s *Store) UpdateMediaImportBatch(ctx context.Context, row mediaImportBatchRow) error {
	_, err := s.pool.Exec(
		ctx,
		`UPDATE media_import_batches
		 SET status = $2,
			 source_type = $3,
			 original_filename = $4,
			 total_items = $5,
			 matched_items = $6,
			 ambiguous_items = $7,
			 unmatched_items = $8,
			 failed_items = $9,
			 imported_items = $10,
			 created_at = $11,
			 updated_at = $12,
			 processed_at = $13,
			 deleted_at = $14
		 WHERE id = $1`,
		row.ID,
		row.Status,
		row.SourceType,
		row.OriginalFile,
		row.TotalItems,
		row.MatchedItems,
		row.AmbiguousItems,
		row.UnmatchedItems,
		row.FailedItems,
		row.ImportedItems,
		row.CreatedAt,
		row.UpdatedAt,
		row.ProcessedAt,
		row.DeletedAt,
	)
	return err
}

func (s *Store) GetMediaImportBatchByID(ctx context.Context, batchID string) (*mediaImportBatchRow, error) {
	row := mediaImportBatchRow{}
	err := s.pool.QueryRow(
		ctx,
		`SELECT id, status, source_type, original_filename, total_items, matched_items, ambiguous_items,
		        unmatched_items, failed_items, imported_items, created_at, updated_at, processed_at, deleted_at
		   FROM media_import_batches
		  WHERE id = $1 AND deleted_at IS NULL
		  LIMIT 1`,
		batchID,
	).Scan(
		&row.ID,
		&row.Status,
		&row.SourceType,
		&row.OriginalFile,
		&row.TotalItems,
		&row.MatchedItems,
		&row.AmbiguousItems,
		&row.UnmatchedItems,
		&row.FailedItems,
		&row.ImportedItems,
		&row.CreatedAt,
		&row.UpdatedAt,
		&row.ProcessedAt,
		&row.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (s *Store) InsertMediaImportBatchItems(ctx context.Context, items []mediaImportBatchItemRow) error {
	for _, item := range items {
		_, err := s.pool.Exec(
			ctx,
			`INSERT INTO media_import_batch_items (
				id, batch_id, original_filename, temp_storage_bucket, temp_storage_key, mime_type, file_size,
				guessed_student_id, guessed_student_name, match_method, match_score, confirmed_student_id,
				media_type, title, description, status, error_message, final_media_id, created_at, updated_at, deleted_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
			)`,
			item.ID,
			item.BatchID,
			item.OriginalFilename,
			item.TempStorageBucket,
			item.TempStorageKey,
			item.MimeType,
			item.FileSize,
			item.GuessedStudentID,
			item.GuessedStudentName,
			item.MatchMethod,
			item.MatchScore,
			item.ConfirmedStudentID,
			item.MediaType,
			item.Title,
			item.Description,
			item.Status,
			item.ErrorMessage,
			item.FinalMediaID,
			item.CreatedAt,
			item.UpdatedAt,
			item.DeletedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListMediaImportBatchItems(ctx context.Context, batchID string) ([]mediaImportBatchItemRow, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, batch_id, original_filename, temp_storage_bucket, temp_storage_key, mime_type, file_size,
		        guessed_student_id, guessed_student_name, match_method, match_score, confirmed_student_id,
				media_type, title, description, status, error_message, final_media_id, created_at, updated_at, deleted_at
		   FROM media_import_batch_items
		  WHERE batch_id = $1 AND deleted_at IS NULL
		  ORDER BY created_at ASC`,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]mediaImportBatchItemRow, 0)
	for rows.Next() {
		var row mediaImportBatchItemRow
		if err := rows.Scan(
			&row.ID,
			&row.BatchID,
			&row.OriginalFilename,
			&row.TempStorageBucket,
			&row.TempStorageKey,
			&row.MimeType,
			&row.FileSize,
			&row.GuessedStudentID,
			&row.GuessedStudentName,
			&row.MatchMethod,
			&row.MatchScore,
			&row.ConfirmedStudentID,
			&row.MediaType,
			&row.Title,
			&row.Description,
			&row.Status,
			&row.ErrorMessage,
			&row.FinalMediaID,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.DeletedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}

	return result, rows.Err()
}

func (s *Store) UpdateMediaImportBatchItem(ctx context.Context, item mediaImportBatchItemRow) error {
	_, err := s.pool.Exec(
		ctx,
		`UPDATE media_import_batch_items
			SET original_filename = $3,
				temp_storage_bucket = $4,
				temp_storage_key = $5,
				mime_type = $6,
				file_size = $7,
				guessed_student_id = $8,
				guessed_student_name = $9,
				match_method = $10,
				match_score = $11,
				confirmed_student_id = $12,
				media_type = $13,
				title = $14,
				description = $15,
				status = $16,
				error_message = $17,
				final_media_id = $18,
				created_at = $19,
				updated_at = $20,
				deleted_at = $21
		  WHERE id = $1 AND batch_id = $2`,
		item.ID,
		item.BatchID,
		item.OriginalFilename,
		item.TempStorageBucket,
		item.TempStorageKey,
		item.MimeType,
		item.FileSize,
		item.GuessedStudentID,
		item.GuessedStudentName,
		item.MatchMethod,
		item.MatchScore,
		item.ConfirmedStudentID,
		item.MediaType,
		item.Title,
		item.Description,
		item.Status,
		item.ErrorMessage,
		item.FinalMediaID,
		item.CreatedAt,
		item.UpdatedAt,
		item.DeletedAt,
	)
	return err
}
