package postgres

import (
	"encoding/json"
	"time"

	"pqq/be/internal/postgres/db"
	"pqq/be/internal/sync"

	"github.com/jackc/pgx/v5/pgtype"
)

func formatTimestamp(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format(time.RFC3339Nano)
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func formatDatePtr(value pgtype.Date) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format("2006-01-02")
	return &formatted
}

func formatTimestampPtr(value pgtype.Timestamptz) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func clubRecordFromRow(row db.Club) sync.ClubRecord {
	return sync.ClubRecord{
		BaseRecord: sync.BaseRecord{
			ID:             row.ID,
			CreatedAt:      formatTimestamp(row.CreatedAt),
			UpdatedAt:      formatTimestamp(row.UpdatedAt),
			LastModifiedAt: formatTimestamp(row.LastModifiedAt),
			DeletedAt:      formatTimestampPtr(row.DeletedAt),
			SyncStatus:     "synced",
		},
		Code:     textPtr(row.Code),
		Name:     row.Name,
		Phone:    textPtr(row.Phone),
		Email:    textPtr(row.Email),
		Address:  textPtr(row.Address),
		Notes:    textPtr(row.Notes),
		IsActive: row.IsActive,
	}
}

func clubGroupRecordFromRow(row db.ClubGroup) sync.ClubGroupRecord {
	return sync.ClubGroupRecord{
		BaseRecord: sync.BaseRecord{
			ID:             row.ID,
			CreatedAt:      formatTimestamp(row.CreatedAt),
			UpdatedAt:      formatTimestamp(row.UpdatedAt),
			LastModifiedAt: formatTimestamp(row.LastModifiedAt),
			DeletedAt:      formatTimestampPtr(row.DeletedAt),
			SyncStatus:     "synced",
		},
		ClubID:      row.ClubID,
		Name:        row.Name,
		Description: textPtr(row.Description),
		IsActive:    row.IsActive,
	}
}

func clubScheduleRecordFromRow(row db.ClubSchedule) sync.ClubScheduleRecord {
	return sync.ClubScheduleRecord{
		BaseRecord: sync.BaseRecord{
			ID:             row.ID,
			CreatedAt:      formatTimestamp(row.CreatedAt),
			UpdatedAt:      formatTimestamp(row.UpdatedAt),
			LastModifiedAt: formatTimestamp(row.LastModifiedAt),
			DeletedAt:      formatTimestampPtr(row.DeletedAt),
			SyncStatus:     "synced",
		},
		ClubID:   row.ClubID,
		Weekday:  row.Weekday,
		IsActive: row.IsActive,
	}
}

func beltRankRecordFromRow(row db.BeltRank) sync.BeltRankRecord {
	return sync.BeltRankRecord{
		BaseRecord: sync.BaseRecord{
			ID:             row.ID,
			CreatedAt:      formatTimestamp(row.CreatedAt),
			UpdatedAt:      formatTimestamp(row.UpdatedAt),
			LastModifiedAt: formatTimestamp(row.LastModifiedAt),
			DeletedAt:      formatTimestampPtr(row.DeletedAt),
			SyncStatus:     "synced",
		},
		Name:        row.Name,
		Order:       int(row.RankOrder),
		Description: textPtr(row.Description),
		IsActive:    row.IsActive,
	}
}

func studentRecordFromRow(row db.Student) sync.StudentRecord {
	return sync.StudentRecord{
		BaseRecord: sync.BaseRecord{
			ID:             row.ID,
			CreatedAt:      formatTimestamp(row.CreatedAt),
			UpdatedAt:      formatTimestamp(row.UpdatedAt),
			LastModifiedAt: formatTimestamp(row.LastModifiedAt),
			DeletedAt:      formatTimestampPtr(row.DeletedAt),
			SyncStatus:     "synced",
		},
		StudentCode: textPtr(row.StudentCode),
		FullName:    row.FullName,
		DateOfBirth: formatDatePtr(row.DateOfBirth),
		Gender:      textPtr(row.Gender),
		Phone:       textPtr(row.Phone),
		Email:       textPtr(row.Email),
		Address:     textPtr(row.Address),
		ClubID:      row.ClubID,
		GroupID:     textPtr(row.GroupID),
		BeltRankID:  row.BeltRankID,
		JoinedAt:    formatDatePtr(row.JoinedAt),
		Status:      row.Status,
		Notes:       textPtr(row.Notes),
	}
}

func studentScheduleProfileRecordFromRow(row db.StudentScheduleProfile) sync.StudentScheduleProfileRecord {
	return sync.StudentScheduleProfileRecord{
		BaseRecord: sync.BaseRecord{
			ID:             row.ID,
			CreatedAt:      formatTimestamp(row.CreatedAt),
			UpdatedAt:      formatTimestamp(row.UpdatedAt),
			LastModifiedAt: formatTimestamp(row.LastModifiedAt),
			DeletedAt:      formatTimestampPtr(row.DeletedAt),
			SyncStatus:     "synced",
		},
		StudentID: row.StudentID,
		Mode:      row.Mode,
	}
}

func studentScheduleRecordFromRow(row db.StudentSchedule) sync.StudentScheduleRecord {
	return sync.StudentScheduleRecord{
		BaseRecord: sync.BaseRecord{
			ID:             row.ID,
			CreatedAt:      formatTimestamp(row.CreatedAt),
			UpdatedAt:      formatTimestamp(row.UpdatedAt),
			LastModifiedAt: formatTimestamp(row.LastModifiedAt),
			DeletedAt:      formatTimestampPtr(row.DeletedAt),
			SyncStatus:     "synced",
		},
		StudentID: row.StudentID,
		Weekday:   row.Weekday,
		IsActive:  row.IsActive,
	}
}

func attendanceSessionRecordFromRow(row db.AttendanceSession) sync.AttendanceSessionRecord {
	return sync.AttendanceSessionRecord{
		BaseRecord: sync.BaseRecord{
			ID:             row.ID,
			CreatedAt:      formatTimestamp(row.CreatedAt),
			UpdatedAt:      formatTimestamp(row.UpdatedAt),
			LastModifiedAt: formatTimestamp(row.LastModifiedAt),
			DeletedAt:      formatTimestampPtr(row.DeletedAt),
			SyncStatus:     "synced",
		},
		ClubID:      row.ClubID,
		SessionDate: row.SessionDate.Time.UTC().Format("2006-01-02"),
		Status:      row.Status,
		Notes:       textPtr(row.Notes),
	}
}

func attendanceRecordFromRow(row db.AttendanceRecord) sync.AttendanceRecord {
	return sync.AttendanceRecord{
		BaseRecord: sync.BaseRecord{
			ID:             row.ID,
			CreatedAt:      formatTimestamp(row.CreatedAt),
			UpdatedAt:      formatTimestamp(row.UpdatedAt),
			LastModifiedAt: formatTimestamp(row.LastModifiedAt),
			DeletedAt:      formatTimestampPtr(row.DeletedAt),
			SyncStatus:     "synced",
		},
		SessionID:        row.SessionID,
		StudentID:        row.StudentID,
		AttendanceStatus: row.AttendanceStatus,
		CheckInAt:        formatTimestampPtr(row.CheckInAt),
		Notes:            textPtr(row.Notes),
	}
}

func storedRecordFromSyncRecord[T any](entityName sync.EntityName, recordID string, deletedAt *string, lastModifiedAt string, payloadRecord T) (*sync.StoredRecord, error) {
	payload, err := json.Marshal(payloadRecord)
	if err != nil {
		return nil, err
	}

	return &sync.StoredRecord{
		EntityName:       entityName,
		RecordID:         recordID,
		Payload:          payload,
		DeletedAt:        deletedAt,
		LastModifiedAt:   lastModifiedAt,
		ServerModifiedAt: lastModifiedAt,
	}, nil
}
