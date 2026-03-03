package postgres

import (
	"encoding/json"
	"time"

	"pqq/be/internal/postgres/db"
	"pqq/be/internal/sync"
)

func formatTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func formatDatePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format("2006-01-02")
	return &formatted
}

func formatTimestampPtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339Nano)
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
		Code:     row.Code,
		Name:     row.Name,
		Phone:    row.Phone,
		Email:    row.Email,
		Address:  row.Address,
		Notes:    row.Notes,
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
		Description: row.Description,
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
		Description: row.Description,
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
		StudentCode: row.StudentCode,
		FullName:    row.FullName,
		DateOfBirth: formatDatePtr(row.DateOfBirth),
		Gender:      row.Gender,
		Phone:       row.Phone,
		Email:       row.Email,
		Address:     row.Address,
		ClubID:      row.ClubID,
		GroupID:     row.GroupID,
		BeltRankID:  row.BeltRankID,
		JoinedAt:    formatDatePtr(row.JoinedAt),
		Status:      row.Status,
		Notes:       row.Notes,
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
		SessionDate: row.SessionDate.UTC().Format("2006-01-02"),
		Status:      row.Status,
		Notes:       row.Notes,
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
		Notes:            row.Notes,
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
