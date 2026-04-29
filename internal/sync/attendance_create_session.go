package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"pqq/be/internal/auth"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Service) CreateAttendanceSession(
	ctx context.Context,
	request CreateAttendanceSessionRequest,
) (CreateAttendanceSessionResponse, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.Subject == "" {
		return CreateAttendanceSessionResponse{}, errors.New("unauthorized")
	}

	clubID := strings.TrimSpace(request.ClubID)
	if clubID == "" {
		return CreateAttendanceSessionResponse{}, errors.New("clubId is required")
	}

	sessionDate := strings.TrimSpace(request.SessionDate)
	if sessionDate == "" {
		return CreateAttendanceSessionResponse{}, errors.New("sessionDate is required")
	}
	if _, err := parseAttendanceSessionDate(sessionDate); err != nil {
		return CreateAttendanceSessionResponse{}, err
	}

	permissions, err := s.authorizer.GetClubPermissions(ctx, claims.Subject, clubID)
	if err != nil {
		return CreateAttendanceSessionResponse{}, err
	}
	if !permissions.Permissions[auth.PermissionAttendanceWrite] {
		return CreateAttendanceSessionResponse{}, errors.New("forbidden")
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return CreateAttendanceSessionResponse{}, err
	}
	defer tx.Rollback(ctx)

	clubRecord, err := s.store.GetRecordForUpdate(ctx, tx, EntityClubs, clubID)
	if err != nil {
		return CreateAttendanceSessionResponse{}, err
	}
	if clubRecord == nil || clubRecord.DeletedAt != nil {
		return CreateAttendanceSessionResponse{}, errors.New("club does not exist")
	}

	var club ClubRecord
	if err := json.Unmarshal(clubRecord.Payload, &club); err != nil {
		return CreateAttendanceSessionResponse{}, err
	}
	if !club.IsActive {
		return CreateAttendanceSessionResponse{}, errors.New("club is inactive")
	}

	existingSession, err := s.store.FindAttendanceSessionByClubAndDate(ctx, tx, clubID, sessionDate)
	if err != nil {
		return CreateAttendanceSessionResponse{}, err
	}
	if existingSession != nil && existingSession.DeletedAt == nil {
		return CreateAttendanceSessionResponse{}, errors.New("attendance session already exists for this date")
	}

	serverNow := time.Now().UTC().Format(time.RFC3339Nano)
	session, records, err := s.buildAttendanceSessionSnapshot(ctx, tx, clubID, sessionDate, request.Notes, serverNow)
	if err != nil {
		return CreateAttendanceSessionResponse{}, err
	}

	sessionPayload, err := json.Marshal(session)
	if err != nil {
		return CreateAttendanceSessionResponse{}, err
	}
	sessionStoredRecord := StoredRecord{
		EntityName:       EntityAttendanceSessions,
		RecordID:         session.ID,
		Payload:          sessionPayload,
		LastModifiedAt:   serverNow,
		ServerModifiedAt: serverNow,
	}
	if err := s.store.UpsertRecord(ctx, tx, sessionStoredRecord); err != nil {
		return CreateAttendanceSessionResponse{}, err
	}

	attendanceRecords := make([]StoredRecord, 0, len(records))
	for _, record := range records {
		payload, err := json.Marshal(record)
		if err != nil {
			return CreateAttendanceSessionResponse{}, err
		}
		attendanceRecords = append(attendanceRecords, StoredRecord{
			EntityName:       EntityAttendanceRecords,
			RecordID:         record.ID,
			Payload:          payload,
			LastModifiedAt:   serverNow,
			ServerModifiedAt: serverNow,
		})
	}
	if err := s.store.UpsertAttendanceRecordsBatch(ctx, tx, attendanceRecords); err != nil {
		return CreateAttendanceSessionResponse{}, err
	}

	if err := s.insertCreateAttendanceSessionAuditLog(
		ctx,
		tx,
		claims.Subject,
		session,
		len(records),
	); err != nil {
		return CreateAttendanceSessionResponse{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateAttendanceSessionResponse{}, err
	}

	recordIDs := make([]string, 0, len(records)+1)
	recordIDs = append(recordIDs, session.ID)
	changedEntities := make([]EntityName, 0, len(records)+1)
	changedEntities = append(changedEntities, EntityAttendanceSessions)
	for _, record := range records {
		recordIDs = append(recordIDs, record.ID)
		changedEntities = append(changedEntities, EntityAttendanceRecords)
	}
	s.hub.BroadcastChange(changedEntities, recordIDs)

	return CreateAttendanceSessionResponse{
		ServerTime: serverNow,
		Session:    session,
		Records:    records,
	}, nil
}

func (s *Service) buildAttendanceSessionSnapshot(
	ctx context.Context,
	tx pgx.Tx,
	clubID string,
	sessionDate string,
	notes *string,
	serverNow string,
) (AttendanceSessionRecord, []AttendanceRecord, error) {
	activeStudents, err := s.store.ListActiveStudentsByClub(ctx, tx, clubID)
	if err != nil {
		return AttendanceSessionRecord{}, nil, err
	}

	expectedStudents, err := s.listExpectedAttendanceStudents(ctx, tx, clubID, sessionDate, activeStudents)
	if err != nil {
		return AttendanceSessionRecord{}, nil, err
	}

	normalizedNotes := normalizeOptionalString(notes)
	session := AttendanceSessionRecord{
		BaseRecord: BaseRecord{
			ID:             uuid.NewString(),
			CreatedAt:      serverNow,
			UpdatedAt:      serverNow,
			LastModifiedAt: serverNow,
			SyncStatus:     "synced",
		},
		ClubID:      clubID,
		SessionDate: sessionDate,
		Status:      "draft",
		Notes:       normalizedNotes,
	}

	records := make([]AttendanceRecord, 0, len(expectedStudents))
	for _, student := range expectedStudents {
		records = append(records, AttendanceRecord{
			BaseRecord: BaseRecord{
				ID:             uuid.NewString(),
				CreatedAt:      serverNow,
				UpdatedAt:      serverNow,
				LastModifiedAt: serverNow,
				SyncStatus:     "synced",
			},
			SessionID:        session.ID,
			StudentID:        student.ID,
			AttendanceStatus: "unmarked",
		})
	}

	return session, records, nil
}

func (s *Service) listExpectedAttendanceStudents(
	ctx context.Context,
	tx pgx.Tx,
	clubID string,
	sessionDate string,
	activeStudents []StudentRecord,
) ([]StudentRecord, error) {
	if len(activeStudents) == 0 {
		return []StudentRecord{}, nil
	}

	weekday, err := parseAttendanceSessionDate(sessionDate)
	if err != nil {
		return nil, err
	}

	clubSchedules, err := s.store.ListActiveClubSchedulesByClub(ctx, tx, clubID)
	if err != nil {
		return nil, err
	}
	if len(clubSchedules) == 0 {
		return activeStudents, nil
	}

	clubWeekdaySet := make(map[string]struct{}, len(clubSchedules))
	for _, row := range clubSchedules {
		clubWeekdaySet[row.Weekday] = struct{}{}
	}
	if _, ok := clubWeekdaySet[weekday]; !ok {
		return []StudentRecord{}, nil
	}

	studentIDs := make([]string, 0, len(activeStudents))
	for _, student := range activeStudents {
		studentIDs = append(studentIDs, student.ID)
	}

	profiles, err := s.store.ListActiveStudentScheduleProfilesByStudentIDs(ctx, tx, studentIDs)
	if err != nil {
		return nil, err
	}
	profileModeByStudentID := make(map[string]string, len(profiles))
	for _, profile := range profiles {
		profileModeByStudentID[profile.StudentID] = profile.Mode
	}

	schedules, err := s.store.ListActiveStudentSchedulesByStudentIDs(ctx, tx, studentIDs)
	if err != nil {
		return nil, err
	}
	customWeekdaysByStudentID := make(map[string]map[string]struct{})
	for _, schedule := range schedules {
		weekdaySet := customWeekdaysByStudentID[schedule.StudentID]
		if weekdaySet == nil {
			weekdaySet = make(map[string]struct{})
			customWeekdaysByStudentID[schedule.StudentID] = weekdaySet
		}
		weekdaySet[schedule.Weekday] = struct{}{}
	}

	expectedStudents := make([]StudentRecord, 0, len(activeStudents))
	for _, student := range activeStudents {
		mode := profileModeByStudentID[student.ID]
		if mode == "" {
			mode = "inherit"
		}

		if mode == "custom" {
			if _, ok := customWeekdaysByStudentID[student.ID][weekday]; ok {
				expectedStudents = append(expectedStudents, student)
			}
			continue
		}

		expectedStudents = append(expectedStudents, student)
	}

	slices.SortFunc(expectedStudents, func(left StudentRecord, right StudentRecord) int {
		return strings.Compare(left.FullName, right.FullName)
	})

	return expectedStudents, nil
}

func (s *Service) insertCreateAttendanceSessionAuditLog(
	ctx context.Context,
	tx pgx.Tx,
	actorUserID string,
	session AttendanceSessionRecord,
	recordCount int,
) error {
	metadata, err := json.Marshal(map[string]any{
		"source":      "api",
		"sessionDate": session.SessionDate,
		"recordCount": recordCount,
	})
	if err != nil {
		return err
	}

	sessionPayload, err := json.Marshal(session)
	if err != nil {
		return err
	}

	entityID := session.ID
	clubID := session.ClubID
	return s.store.InsertAuditLog(
		ctx,
		tx,
		&actorUserID,
		&clubID,
		string(EntityAttendanceSessions),
		&entityID,
		"create",
		nil,
		sessionPayload,
		metadata,
	)
}

func parseAttendanceSessionDate(value string) (string, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", errors.New("sessionDate must be YYYY-MM-DD")
	}

	switch parsed.Weekday() {
	case time.Monday:
		return "mon", nil
	case time.Tuesday:
		return "tue", nil
	case time.Wednesday:
		return "wed", nil
	case time.Thursday:
		return "thu", nil
	case time.Friday:
		return "fri", nil
	case time.Saturday:
		return "sat", nil
	case time.Sunday:
		return "sun", nil
	default:
		return "", fmt.Errorf("unsupported session weekday for %s", value)
	}
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}
