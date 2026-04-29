package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"pqq/be/internal/auth"

	"github.com/jackc/pgx/v5"
)

type attendanceCreateSessionPayload struct {
	Session AttendanceSessionRecord `json:"session"`
	Records []AttendanceRecord      `json:"records"`
}

type attendanceSetRecordStatusPayload struct {
	AttendanceStatus string  `json:"attendanceStatus"`
	CheckInAt        *string `json:"checkInAt,omitempty"`
}

type attendanceMarkAllPresentPayload struct {
	RecordIDs []string `json:"recordIds"`
	CheckInAt *string  `json:"checkInAt,omitempty"`
}

type attendanceSetRecordNotePayload struct {
	Notes *string `json:"notes,omitempty"`
}

type attendanceSetSessionNotePayload struct {
	Notes *string `json:"notes,omitempty"`
}

type attendanceSetSessionStatusPayload struct {
	Status string `json:"status"`
}

type attendanceDeleteSessionPayload struct {
	DeletedAt *string `json:"deletedAt,omitempty"`
}

func (s *Service) PushAttendanceActions(
	ctx context.Context,
	request AttendanceActionPushRequest,
) (AttendanceActionPushResponse, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.Subject == "" {
		return AttendanceActionPushResponse{}, errors.New("unauthorized")
	}
	if request.DeviceID == "" {
		return AttendanceActionPushResponse{}, errors.New("deviceId is required")
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return AttendanceActionPushResponse{}, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	response := AttendanceActionPushResponse{
		ServerTime:       now,
		AppliedActionIDs: make([]string, 0, len(request.Actions)),
		Changes:          make([]AttendanceActionAppliedChange, 0),
		Errors:           make([]AttendanceActionError, 0),
	}
	changedEntities := make([]EntityName, 0)
	changedIDs := make([]string, 0)

	for _, action := range request.Actions {
		if err := validateAttendanceAction(action); err != nil {
			return AttendanceActionPushResponse{}, err
		}

		processed, err := s.store.IsMutationProcessed(ctx, tx, request.DeviceID, action.ActionID)
		if err != nil {
			return AttendanceActionPushResponse{}, err
		}
		if processed {
			response.AppliedActionIDs = append(response.AppliedActionIDs, action.ActionID)
			continue
		}

		if err := s.authorizeAttendanceAction(ctx, claims, action); err != nil {
			if conflict, ok := err.(conflictError); ok {
				response.Errors = append(response.Errors, attendanceActionErrorFromConflict(action, conflict))
				continue
			}
			return AttendanceActionPushResponse{}, err
		}

		appliedRecords, err := s.applyAttendanceAction(ctx, tx, action, now)
		if err != nil {
			if conflict, ok := err.(conflictError); ok {
				response.Errors = append(response.Errors, attendanceActionErrorFromConflict(action, conflict))
				continue
			}
			return AttendanceActionPushResponse{}, err
		}

		targetEntity, targetRecordID := attendanceProcessedMutationTarget(action)
		if err := s.store.SaveProcessedMutation(ctx, tx, request.DeviceID, SyncMutation{
			MutationID:       action.ActionID,
			EntityName:       targetEntity,
			Operation:        OperationUpsert,
			RecordID:         targetRecordID,
			ClientModifiedAt: action.ClientOccurredAt,
		}, now); err != nil {
			return AttendanceActionPushResponse{}, err
		}

		response.AppliedActionIDs = append(response.AppliedActionIDs, action.ActionID)
		includeChangesInResponse :=
			action.ActionType != AttendanceActionCreateSession &&
				action.ActionType != AttendanceActionMarkAllPresent
		for _, record := range appliedRecords {
			if includeChangesInResponse {
				response.Changes = append(response.Changes, AttendanceActionAppliedChange{
					EntityName:       record.EntityName,
					Record:           json.RawMessage(record.Payload),
					ServerModifiedAt: record.ServerModifiedAt,
				})
			}
			changedEntities = append(changedEntities, record.EntityName)
			changedIDs = append(changedIDs, record.RecordID)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return AttendanceActionPushResponse{}, err
	}

	if len(changedIDs) > 0 {
		s.hub.BroadcastChange(changedEntities, changedIDs)
	}

	return response, nil
}

func (s *Service) authorizeAttendanceAction(
	ctx context.Context,
	claims *auth.Claims,
	action AttendanceActionMutation,
) error {
	permissions, err := s.authorizer.GetClubPermissions(ctx, claims.Subject, action.ClubID)
	if err != nil {
		return err
	}
	if permissions.Permissions[auth.PermissionAttendanceWrite] {
		return nil
	}
	return conflictError{
		reason:  "forbidden",
		message: "You do not have permission to update attendance for this club.",
	}
}

func (s *Service) applyAttendanceAction(
	ctx context.Context,
	tx pgx.Tx,
	action AttendanceActionMutation,
	serverNow string,
) ([]StoredRecord, error) {
	switch action.ActionType {
	case AttendanceActionCreateSession:
		return s.applyAttendanceCreateSessionAction(ctx, tx, action, serverNow)
	case AttendanceActionMarkAllPresent:
		return s.applyAttendanceMarkAllPresentAction(ctx, tx, action, serverNow)
	case AttendanceActionSetRecordStatus:
		return s.applyAttendanceSetRecordStatusAction(ctx, tx, action, serverNow)
	case AttendanceActionSetRecordNote:
		return s.applyAttendanceSetRecordNoteAction(ctx, tx, action, serverNow)
	case AttendanceActionSetSessionNote:
		return s.applyAttendanceSetSessionNoteAction(ctx, tx, action, serverNow)
	case AttendanceActionSetSessionStatus:
		return s.applyAttendanceSetSessionStatusAction(ctx, tx, action, serverNow)
	case AttendanceActionDeleteSession:
		return s.applyAttendanceDeleteSessionAction(ctx, tx, action, serverNow)
	default:
		return nil, errors.New("attendance action type is invalid")
	}
}

func (s *Service) applyAttendanceCreateSessionAction(
	ctx context.Context,
	tx pgx.Tx,
	action AttendanceActionMutation,
	serverNow string,
) ([]StoredRecord, error) {
	var payload attendanceCreateSessionPayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return nil, err
	}
	if payload.Session.ID != action.SessionID {
		return nil, conflictError{reason: "validation_failed", message: "Session payload does not match the action session."}
	}
	if payload.Session.ClubID != action.ClubID {
		return nil, conflictError{reason: "validation_failed", message: "Session payload does not match the action club."}
	}

	existingSession, err := s.store.GetRecordForUpdate(ctx, tx, EntityAttendanceSessions, action.SessionID)
	if err != nil {
		return nil, err
	}
	if existingSession != nil && existingSession.DeletedAt == nil {
		return nil, conflictError{
			reason:       "duplicate_value",
			message:      "Attendance session already exists.",
			serverRecord: existingSession.Payload,
		}
	}

	sessionMutation, err := marshalSyncMutation(
		action.ActionID,
		EntityAttendanceSessions,
		OperationUpsert,
		action.SessionID,
		payload.Session,
		action.ClientOccurredAt,
	)
	if err != nil {
		return nil, err
	}
	sessionRecord, err := s.upsertAttendanceActionMutation(ctx, tx, sessionMutation, existingSession, serverNow)
	if err != nil {
		return nil, err
	}
	if err := s.writeAttendanceActionAuditLog(ctx, tx, action, payload.Session, existingSession, sessionRecord); err != nil {
		return nil, err
	}

	applied := []StoredRecord{sessionRecord}
	attendanceRecords := make([]StoredRecord, 0, len(payload.Records))
	for _, record := range payload.Records {
		if record.SessionID != action.SessionID {
			return nil, conflictError{reason: "validation_failed", message: "Attendance record session does not match the action session."}
		}
		recordMutation, err := marshalSyncMutation(
			fmt.Sprintf("%s:%s", action.ActionID, record.ID),
			EntityAttendanceRecords,
			OperationUpsert,
			record.ID,
			record,
			action.ClientOccurredAt,
		)
		if err != nil {
			return nil, err
		}
		canonicalPayload, deletedAt, err := s.canonicalizeMutation(ctx, tx, recordMutation, serverNow, nil)
		if err != nil {
			return nil, err
		}
		attendanceRecords = append(attendanceRecords, StoredRecord{
			EntityName:       EntityAttendanceRecords,
			RecordID:         record.ID,
			Payload:          canonicalPayload,
			DeletedAt:        deletedAt,
			LastModifiedAt:   serverNow,
			ServerModifiedAt: serverNow,
		})
	}

	if err := s.store.UpsertAttendanceRecordsBatch(ctx, tx, attendanceRecords); err != nil {
		return nil, err
	}
	applied = append(applied, attendanceRecords...)

	return applied, nil
}

func (s *Service) applyAttendanceSetRecordStatusAction(
	ctx context.Context,
	tx pgx.Tx,
	action AttendanceActionMutation,
	serverNow string,
) ([]StoredRecord, error) {
	record, existing, session, err := s.loadAttendanceRecordActionTarget(ctx, tx, action)
	if err != nil {
		return nil, err
	}

	var payload attendanceSetRecordStatusPayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return nil, err
	}

	record.AttendanceStatus = payload.AttendanceStatus
	record.CheckInAt = payload.CheckInAt

	mutation, err := marshalSyncMutation(
		action.ActionID,
		EntityAttendanceRecords,
		OperationUpsert,
		record.ID,
		record,
		action.ClientOccurredAt,
	)
	if err != nil {
		return nil, err
	}
	appliedRecord, err := s.upsertAttendanceActionMutation(ctx, tx, mutation, existing, serverNow)
	if err != nil {
		return nil, err
	}
	if err := s.syncAttendanceRecordMessage(ctx, tx, action, session, record, serverNow); err != nil {
		return nil, err
	}

	if err := s.writeAttendanceActionAuditLog(ctx, tx, action, session, existing, appliedRecord); err != nil {
		return nil, err
	}

	return []StoredRecord{appliedRecord}, nil
}

func (s *Service) applyAttendanceMarkAllPresentAction(
	ctx context.Context,
	tx pgx.Tx,
	action AttendanceActionMutation,
	serverNow string,
) ([]StoredRecord, error) {
	session, _, err := s.loadAttendanceSessionActionTarget(ctx, tx, action)
	if err != nil {
		return nil, err
	}

	var payload attendanceMarkAllPresentPayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return nil, err
	}
	if len(payload.RecordIDs) == 0 {
		return []StoredRecord{}, nil
	}

	targetRecordIDs := make(map[string]struct{}, len(payload.RecordIDs))
	for _, recordID := range payload.RecordIDs {
		if recordID == "" {
			return nil, conflictError{reason: "validation_failed", message: "Attendance record id is required."}
		}
		targetRecordIDs[recordID] = struct{}{}
	}

	recordRows, err := s.store.ListAttendanceRecordsBySessionForUpdate(ctx, tx, session.ID)
	if err != nil {
		return nil, err
	}

	applied := make([]StoredRecord, 0, len(targetRecordIDs))
	for _, existingRecord := range recordRows {
		if _, ok := targetRecordIDs[existingRecord.RecordID]; !ok {
			continue
		}

		var record AttendanceRecord
		if err := json.Unmarshal(existingRecord.Payload, &record); err != nil {
			return nil, err
		}
		record.AttendanceStatus = "present"
		record.CheckInAt = payload.CheckInAt

		recordMutation, err := marshalSyncMutation(
			fmt.Sprintf("%s:%s", action.ActionID, record.ID),
			EntityAttendanceRecords,
			OperationUpsert,
			record.ID,
			record,
			action.ClientOccurredAt,
		)
		if err != nil {
			return nil, err
		}
		canonicalPayload, deletedAt, err := s.canonicalizeMutation(ctx, tx, recordMutation, serverNow, &existingRecord)
		if err != nil {
			return nil, err
		}
		applied = append(applied, StoredRecord{
			EntityName:       EntityAttendanceRecords,
			RecordID:         record.ID,
			Payload:          canonicalPayload,
			DeletedAt:        deletedAt,
			LastModifiedAt:   serverNow,
			ServerModifiedAt: serverNow,
		})
		delete(targetRecordIDs, record.ID)
	}

	if len(targetRecordIDs) > 0 {
		return nil, conflictError{
			reason:  "foreign_key_missing",
			message: "Some attendance records do not exist in this session.",
		}
	}

	if err := s.store.UpsertAttendanceRecordsBatch(ctx, tx, applied); err != nil {
		return nil, err
	}

	return applied, nil
}

func (s *Service) applyAttendanceSetRecordNoteAction(
	ctx context.Context,
	tx pgx.Tx,
	action AttendanceActionMutation,
	serverNow string,
) ([]StoredRecord, error) {
	record, existing, session, err := s.loadAttendanceRecordActionTarget(ctx, tx, action)
	if err != nil {
		return nil, err
	}

	var payload attendanceSetRecordNotePayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return nil, err
	}

	record.Notes = payload.Notes

	mutation, err := marshalSyncMutation(
		action.ActionID,
		EntityAttendanceRecords,
		OperationUpsert,
		record.ID,
		record,
		action.ClientOccurredAt,
	)
	if err != nil {
		return nil, err
	}
	appliedRecord, err := s.upsertAttendanceActionMutation(ctx, tx, mutation, existing, serverNow)
	if err != nil {
		return nil, err
	}
	if err := s.syncAttendanceRecordMessage(ctx, tx, action, session, record, serverNow); err != nil {
		return nil, err
	}

	if err := s.writeAttendanceActionAuditLog(ctx, tx, action, session, existing, appliedRecord); err != nil {
		return nil, err
	}

	return []StoredRecord{appliedRecord}, nil
}

func (s *Service) applyAttendanceSetSessionNoteAction(
	ctx context.Context,
	tx pgx.Tx,
	action AttendanceActionMutation,
	serverNow string,
) ([]StoredRecord, error) {
	session, existing, err := s.loadAttendanceSessionActionTarget(ctx, tx, action)
	if err != nil {
		return nil, err
	}

	var payload attendanceSetSessionNotePayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return nil, err
	}
	session.Notes = payload.Notes

	mutation, err := marshalSyncMutation(
		action.ActionID,
		EntityAttendanceSessions,
		OperationUpsert,
		session.ID,
		session,
		action.ClientOccurredAt,
	)
	if err != nil {
		return nil, err
	}
	appliedSession, err := s.upsertAttendanceActionMutation(ctx, tx, mutation, existing, serverNow)
	if err != nil {
		return nil, err
	}

	if err := s.writeAttendanceActionAuditLog(ctx, tx, action, session, existing, appliedSession); err != nil {
		return nil, err
	}

	return []StoredRecord{appliedSession}, nil
}

func (s *Service) applyAttendanceSetSessionStatusAction(
	ctx context.Context,
	tx pgx.Tx,
	action AttendanceActionMutation,
	serverNow string,
) ([]StoredRecord, error) {
	session, existing, err := s.loadAttendanceSessionActionTarget(ctx, tx, action)
	if err != nil {
		return nil, err
	}

	var payload attendanceSetSessionStatusPayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return nil, err
	}
	session.Status = payload.Status

	mutation, err := marshalSyncMutation(
		action.ActionID,
		EntityAttendanceSessions,
		OperationUpsert,
		session.ID,
		session,
		action.ClientOccurredAt,
	)
	if err != nil {
		return nil, err
	}
	appliedSession, err := s.upsertAttendanceActionMutation(ctx, tx, mutation, existing, serverNow)
	if err != nil {
		return nil, err
	}

	if err := s.writeAttendanceActionAuditLog(ctx, tx, action, session, existing, appliedSession); err != nil {
		return nil, err
	}

	return []StoredRecord{appliedSession}, nil
}

func (s *Service) applyAttendanceDeleteSessionAction(
	ctx context.Context,
	tx pgx.Tx,
	action AttendanceActionMutation,
	serverNow string,
) ([]StoredRecord, error) {
	session, existingSession, err := s.loadAttendanceSessionActionTarget(ctx, tx, action)
	if err != nil {
		return nil, err
	}

	var payload attendanceDeleteSessionPayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return nil, err
	}
	_ = payload

	sessionMutation, err := marshalSyncMutation(
		action.ActionID,
		EntityAttendanceSessions,
		OperationDelete,
		session.ID,
		session,
		action.ClientOccurredAt,
	)
	if err != nil {
		return nil, err
	}
	deletedSession, err := s.upsertAttendanceActionMutation(ctx, tx, sessionMutation, existingSession, serverNow)
	if err != nil {
		return nil, err
	}

	recordRows, err := s.store.ListAttendanceRecordsBySessionForUpdate(ctx, tx, session.ID)
	if err != nil {
		return nil, err
	}

	applied := []StoredRecord{deletedSession}
	deletedRecords := make([]StoredRecord, 0, len(recordRows))
	for _, existingRecord := range recordRows {
		var record AttendanceRecord
		if err := json.Unmarshal(existingRecord.Payload, &record); err != nil {
			return nil, err
		}
		recordMutation, err := marshalSyncMutation(
			fmt.Sprintf("%s:%s", action.ActionID, record.ID),
			EntityAttendanceRecords,
			OperationDelete,
			record.ID,
			record,
			action.ClientOccurredAt,
		)
		if err != nil {
			return nil, err
		}
		canonicalPayload, deletedAt, err := s.canonicalizeMutation(ctx, tx, recordMutation, serverNow, &existingRecord)
		if err != nil {
			return nil, err
		}
		deletedRecords = append(deletedRecords, StoredRecord{
			EntityName:       EntityAttendanceRecords,
			RecordID:         record.ID,
			Payload:          canonicalPayload,
			DeletedAt:        deletedAt,
			LastModifiedAt:   serverNow,
			ServerModifiedAt: serverNow,
		})
	}
	if err := s.store.UpsertAttendanceRecordsBatch(ctx, tx, deletedRecords); err != nil {
		return nil, err
	}
	applied = append(applied, deletedRecords...)

	messageRows, err := s.store.ListStudentMessagesByAttendanceSessionForUpdate(ctx, tx, session.ID)
	if err != nil {
		return nil, err
	}
	deletedMessages := make([]StoredRecord, 0, len(messageRows))
	for _, existingMessage := range messageRows {
		var messageRecord StudentMessageRecord
		if err := json.Unmarshal(existingMessage.Payload, &messageRecord); err != nil {
			return nil, err
		}
		deleteMutation, err := marshalSyncMutation(
			fmt.Sprintf("%s:mirror-delete:%s", action.ActionID, messageRecord.ID),
			EntityStudentMessages,
			OperationDelete,
			messageRecord.ID,
			messageRecord,
			action.ClientOccurredAt,
		)
		if err != nil {
			return nil, err
		}
		canonicalPayload, deletedAt, err := s.canonicalizeMutation(ctx, tx, deleteMutation, serverNow, &existingMessage)
		if err != nil {
			return nil, err
		}
		deletedMessages = append(deletedMessages, StoredRecord{
			EntityName:       EntityStudentMessages,
			RecordID:         messageRecord.ID,
			Payload:          canonicalPayload,
			DeletedAt:        deletedAt,
			LastModifiedAt:   serverNow,
			ServerModifiedAt: serverNow,
		})
	}
	if err := s.store.UpsertStudentMessagesBatch(ctx, tx, deletedMessages); err != nil {
		return nil, err
	}

	if err := s.writeAttendanceActionAuditLog(ctx, tx, action, session, existingSession, deletedSession); err != nil {
		return nil, err
	}

	return applied, nil
}

func (s *Service) loadAttendanceSessionActionTarget(
	ctx context.Context,
	tx pgx.Tx,
	action AttendanceActionMutation,
) (AttendanceSessionRecord, *StoredRecord, error) {
	existing, err := s.store.GetRecordForUpdate(ctx, tx, EntityAttendanceSessions, action.SessionID)
	if err != nil {
		return AttendanceSessionRecord{}, nil, err
	}
	if existing == nil {
		return AttendanceSessionRecord{}, nil, conflictError{reason: "foreign_key_missing", message: "Attendance session does not exist."}
	}

	var session AttendanceSessionRecord
	if err := json.Unmarshal(existing.Payload, &session); err != nil {
		return AttendanceSessionRecord{}, nil, err
	}
	if session.ClubID != action.ClubID {
		return AttendanceSessionRecord{}, nil, conflictError{
			reason:       "forbidden",
			message:      "Attendance session does not belong to the selected club.",
			serverRecord: existing.Payload,
		}
	}

	return session, existing, nil
}

func (s *Service) loadAttendanceRecordActionTarget(
	ctx context.Context,
	tx pgx.Tx,
	action AttendanceActionMutation,
) (AttendanceRecord, *StoredRecord, AttendanceSessionRecord, error) {
	if action.RecordID == nil || *action.RecordID == "" {
		return AttendanceRecord{}, nil, AttendanceSessionRecord{}, errors.New("recordId is required")
	}
	if action.StudentID == nil || *action.StudentID == "" {
		return AttendanceRecord{}, nil, AttendanceSessionRecord{}, errors.New("studentId is required")
	}

	existing, err := s.store.GetRecordForUpdate(ctx, tx, EntityAttendanceRecords, *action.RecordID)
	if err != nil {
		return AttendanceRecord{}, nil, AttendanceSessionRecord{}, err
	}
	if existing == nil {
		return AttendanceRecord{}, nil, AttendanceSessionRecord{}, conflictError{reason: "foreign_key_missing", message: "Attendance record does not exist."}
	}

	var record AttendanceRecord
	if err := json.Unmarshal(existing.Payload, &record); err != nil {
		return AttendanceRecord{}, nil, AttendanceSessionRecord{}, err
	}
	if record.SessionID != action.SessionID || record.StudentID != *action.StudentID {
		return AttendanceRecord{}, nil, AttendanceSessionRecord{}, conflictError{
			reason:       "validation_failed",
			message:      "Attendance record does not match the action target.",
			serverRecord: existing.Payload,
		}
	}

	session, _, err := s.loadAttendanceSessionActionTarget(ctx, tx, action)
	if err != nil {
		return AttendanceRecord{}, nil, AttendanceSessionRecord{}, err
	}

	return record, existing, session, nil
}

func (s *Service) upsertAttendanceActionMutation(
	ctx context.Context,
	tx pgx.Tx,
	mutation SyncMutation,
	existing *StoredRecord,
	serverNow string,
) (StoredRecord, error) {
	canonicalPayload, deletedAt, err := s.canonicalizeMutation(ctx, tx, mutation, serverNow, existing)
	if err != nil {
		return StoredRecord{}, err
	}

	record := StoredRecord{
		EntityName:       mutation.EntityName,
		RecordID:         mutation.RecordID,
		Payload:          canonicalPayload,
		DeletedAt:        deletedAt,
		LastModifiedAt:   serverNow,
		ServerModifiedAt: serverNow,
	}
	if err := s.store.UpsertRecord(ctx, tx, record); err != nil {
		return StoredRecord{}, err
	}
	return record, nil
}

func (s *Service) writeAttendanceActionAuditLog(
	ctx context.Context,
	tx pgx.Tx,
	action AttendanceActionMutation,
	session AttendanceSessionRecord,
	existing *StoredRecord,
	applied StoredRecord,
) error {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.Subject == "" {
		return errors.New("unauthorized")
	}
	metadata, err := json.Marshal(map[string]any{
		"actionId":         action.ActionID,
		"actionType":       action.ActionType,
		"clientOccurredAt": action.ClientOccurredAt,
		"sessionId":        action.SessionID,
		"recordId":         action.RecordID,
		"studentId":        action.StudentID,
	})
	if err != nil {
		return err
	}
	operation := OperationUpsert
	if action.ActionType == AttendanceActionDeleteSession {
		operation = OperationDelete
	}
	entityID := applied.RecordID
	clubID := session.ClubID
	actorUserID := claims.Subject
	return s.store.InsertAuditLog(
		ctx,
		tx,
		&actorUserID,
		&clubID,
		string(applied.EntityName),
		&entityID,
		attendanceActionAuditAction(action),
		auditRecordPayload(existing),
		auditNewValues(operation, applied.Payload),
		metadata,
	)
}

func attendanceActionAuditAction(action AttendanceActionMutation) string {
	switch action.ActionType {
	case AttendanceActionCreateSession:
		return "create"
	case AttendanceActionMarkAllPresent:
		return "update_status"
	case AttendanceActionDeleteSession:
		return "delete"
	case AttendanceActionSetSessionStatus:
		return "update_status"
	case AttendanceActionSetSessionNote, AttendanceActionSetRecordNote:
		return "update_note"
	case AttendanceActionSetRecordStatus:
		return "update_status"
	default:
		return "update"
	}
}

func attendanceProcessedMutationTarget(action AttendanceActionMutation) (EntityName, string) {
	switch action.ActionType {
	case AttendanceActionMarkAllPresent:
		return EntityAttendanceSessions, action.SessionID
	case AttendanceActionSetRecordStatus, AttendanceActionSetRecordNote:
		if action.RecordID != nil && *action.RecordID != "" {
			return EntityAttendanceRecords, *action.RecordID
		}
		return EntityAttendanceRecords, action.SessionID
	default:
		return EntityAttendanceSessions, action.SessionID
	}
}

func attendanceActionErrorFromConflict(action AttendanceActionMutation, conflict conflictError) AttendanceActionError {
	attendanceError := AttendanceActionError{
		ActionID:   action.ActionID,
		ActionType: action.ActionType,
		Message:    conflict.message,
		RecordID:   action.RecordID,
		SessionID:  action.SessionID,
		StudentID:  action.StudentID,
	}
	switch action.ActionType {
	case AttendanceActionSetRecordStatus, AttendanceActionSetRecordNote:
		if len(conflict.serverRecord) > 0 {
			attendanceError.ServerRecord = json.RawMessage(conflict.serverRecord)
		}
	default:
		if len(conflict.serverRecord) > 0 {
			attendanceError.ServerSession = json.RawMessage(conflict.serverRecord)
		}
	}
	return attendanceError
}

func marshalSyncMutation(
	mutationID string,
	entityName EntityName,
	operation Operation,
	recordID string,
	record any,
	clientModifiedAt string,
) (SyncMutation, error) {
	payload, err := json.Marshal(record)
	if err != nil {
		return SyncMutation{}, err
	}
	return SyncMutation{
		MutationID:       mutationID,
		EntityName:       entityName,
		Operation:        operation,
		RecordID:         recordID,
		Record:           payload,
		ClientModifiedAt: clientModifiedAt,
	}, nil
}

func validateAttendanceAction(action AttendanceActionMutation) error {
	if action.ActionID == "" {
		return errors.New("actionId is required")
	}
	if action.ClubID == "" {
		return errors.New("clubId is required")
	}
	if action.SessionID == "" {
		return errors.New("sessionId is required")
	}
	if action.ClientOccurredAt == "" {
		return errors.New("clientOccurredAt is required")
	}
	if _, err := time.Parse(time.RFC3339Nano, action.ClientOccurredAt); err != nil {
		return errors.New("clientOccurredAt must be RFC3339 timestamp")
	}
	switch action.ActionType {
	case AttendanceActionCreateSession,
		AttendanceActionMarkAllPresent,
		AttendanceActionSetRecordStatus,
		AttendanceActionSetRecordNote,
		AttendanceActionSetSessionNote,
		AttendanceActionSetSessionStatus,
		AttendanceActionDeleteSession:
	default:
		return errors.New("actionType is invalid")
	}
	return nil
}

func (s *Service) syncAttendanceRecordMessage(
	ctx context.Context,
	tx pgx.Tx,
	action AttendanceActionMutation,
	session AttendanceSessionRecord,
	record AttendanceRecord,
	serverNow string,
) error {
	if action.ActionType != AttendanceActionSetRecordNote {
		return nil
	}

	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.Subject == "" {
		return errors.New("unauthorized")
	}

	messageID := attendanceMessageID(record.ID)
	existingMessage, err := s.store.GetRecordForUpdate(ctx, tx, EntityStudentMessages, messageID)
	if err != nil {
		return err
	}

	if record.Notes == nil || strings.TrimSpace(*record.Notes) == "" {
		if existingMessage == nil {
			return nil
		}
		var existingRecord StudentMessageRecord
		if err := json.Unmarshal(existingMessage.Payload, &existingRecord); err != nil {
			return err
		}
		deleteMutation, err := marshalSyncMutation(
			fmt.Sprintf("%s:mirror-delete", action.ActionID),
			EntityStudentMessages,
			OperationDelete,
			messageID,
			existingRecord,
			action.ClientOccurredAt,
		)
		if err != nil {
			return err
		}
		_, err = s.upsertAttendanceActionMutation(ctx, tx, deleteMutation, existingMessage, serverNow)
		return err
	}

	authorName, err := s.store.ResolveUserFullName(ctx, tx, claims.Subject)
	if err != nil {
		return err
	}
	if authorName == "" {
		authorName = claims.Subject
	}

	messageRecord := StudentMessageRecord{
		BaseRecord: BaseRecord{
			ID: messageID,
		},
		StudentID:             record.StudentID,
		ClubID:                session.ClubID,
		MessageType:           "attendance_note",
		Content:               strings.TrimSpace(*record.Notes),
		AuthorUserID:          stringPtr(claims.Subject),
		AuthorName:            authorName,
		AttendanceSessionID:   stringPtr(session.ID),
		AttendanceRecordID:    stringPtr(record.ID),
		AttendanceSessionDate: stringPtr(session.SessionDate),
		AttendanceStatus:      stringPtr(record.AttendanceStatus),
	}
	if existingMessage != nil {
		var existingRecord StudentMessageRecord
		if err := json.Unmarshal(existingMessage.Payload, &existingRecord); err != nil {
			return err
		}
		messageRecord.CreatedAt = existingRecord.CreatedAt
	}

	messageMutation, err := marshalSyncMutation(
		fmt.Sprintf("%s:mirror", action.ActionID),
		EntityStudentMessages,
		OperationUpsert,
		messageID,
		messageRecord,
		action.ClientOccurredAt,
	)
	if err != nil {
		return err
	}

	_, err = s.upsertAttendanceActionMutation(ctx, tx, messageMutation, existingMessage, serverNow)
	return err
}

func attendanceMessageID(attendanceRecordID string) string {
	return fmt.Sprintf("attendance-note-%s", attendanceRecordID)
}
