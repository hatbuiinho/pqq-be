package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"pqq/be/internal/storage"
)

const (
	avatarImportBatchStatusAnalyzed    = "analyzed"
	avatarImportBatchStatusCompleted   = "completed"
	avatarImportSourceTypeFiles        = "files"
	avatarImportItemStatusMatched      = "matched"
	avatarImportItemStatusAmbiguous    = "ambiguous"
	avatarImportItemStatusUnmatched    = "unmatched"
	avatarImportItemStatusImported     = "imported"
	avatarImportItemStatusFailed       = "failed"
	avatarImportItemStatusSkipped      = "skipped"
	avatarImportReplaceStrategyKeep    = "keep"
	avatarImportReplaceStrategyReplace = "replace"
)

var studentCodeImportPattern = regexp.MustCompile(`(?i)\bPQQ-\d{6}\b`)
var trailingTimestampPattern = regexp.MustCompile(`[-_ ]?\d{8}t\d{6}z$|[-_ ]?\d{8}$`)

type AvatarImportUpload struct {
	Filename    string
	ContentType string
	Size        int64
	Reader      io.Reader
}

type avatarMatchCandidate struct {
	ID                 string
	StudentCode        *string
	FullName           string
	NormalizedFullName string
}

func (s *Service) AnalyzeAvatarImport(ctx context.Context, files []AvatarImportUpload) (*AnalyzeAvatarImportResponse, error) {
	if s.storage == nil {
		return nil, errors.New("storage service is not configured")
	}
	if len(files) == 0 {
		return nil, errors.New("at least one file is required")
	}

	students, err := s.store.ListActiveStudentsForImport(ctx)
	if err != nil {
		return nil, err
	}
	candidates := buildAvatarMatchCandidates(students)

	batchID, err := generateMediaID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	batchRow := mediaImportBatchRow{
		ID:         batchID,
		Status:     avatarImportBatchStatusAnalyzed,
		SourceType: avatarImportSourceTypeFiles,
		TotalItems: len(files),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	itemRows := make([]mediaImportBatchItemRow, 0, len(files))
	for _, file := range files {
		itemID, err := generateMediaID()
		if err != nil {
			return nil, err
		}

		data, err := readAvatarBytes(file.Reader, file.Size)
		if err != nil {
			itemRows = append(itemRows, newFailedImportItemRow(batchID, itemID, file, now, err.Error()))
			batchRow.FailedItems += 1
			continue
		}

		mimeType := httpDetectContentType(data)
		if _, ok := avatarMimeTypes[mimeType]; !ok {
			itemRows = append(itemRows, newFailedImportItemRow(batchID, itemID, file, now, "avatar must be a JPG, PNG, or WebP image"))
			batchRow.FailedItems += 1
			continue
		}

		tempKey := buildAvatarImportTempStorageKey(batchID, itemID, file.Filename)
		uploadedObject, err := s.storage.UploadObject(ctx, storage.UploadObjectInput{
			Key:         tempKey,
			ContentType: mimeType,
			Size:        int64(len(data)),
			Body:        bytes.NewReader(data),
		})
		if err != nil {
			itemRows = append(itemRows, newFailedImportItemRow(batchID, itemID, file, now, err.Error()))
			batchRow.FailedItems += 1
			continue
		}

		match := matchAvatarImportStudent(file.Filename, candidates)
		itemRow := mediaImportBatchItemRow{
			ID:                itemID,
			BatchID:           batchID,
			OriginalFilename:  sanitizeFilename(file.Filename),
			TempStorageBucket: uploadedObject.Bucket,
			TempStorageKey:    uploadedObject.Key,
			MimeType:          mimeType,
			FileSize:          uploadedObject.Size,
			MediaType:         mediaTypeAvatar,
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		switch match.status {
		case avatarImportItemStatusMatched:
			itemRow.Status = avatarImportItemStatusMatched
			itemRow.GuessedStudentID = &match.studentID
			itemRow.ConfirmedStudentID = &match.studentID
			itemRow.MatchMethod = stringPtr(match.method)
			itemRow.MatchScore = intPtr(match.score)
			itemRow.GuessedStudentName = stringPtr(match.studentName)
			batchRow.MatchedItems += 1
		case avatarImportItemStatusAmbiguous:
			itemRow.Status = avatarImportItemStatusAmbiguous
			itemRow.MatchMethod = stringPtr(match.method)
			itemRow.MatchScore = intPtr(match.score)
			itemRow.ErrorMessage = stringPtr("Multiple students matched this filename.")
			batchRow.AmbiguousItems += 1
		default:
			itemRow.Status = avatarImportItemStatusUnmatched
			itemRow.ErrorMessage = stringPtr("No student matched this filename.")
			batchRow.UnmatchedItems += 1
		}

		itemRows = append(itemRows, itemRow)
	}

	if err := s.store.InsertMediaImportBatch(ctx, batchRow); err != nil {
		return nil, err
	}
	if err := s.store.InsertMediaImportBatchItems(ctx, itemRows); err != nil {
		return nil, err
	}

	items, err := s.toAvatarImportBatchItems(ctx, itemRows)
	if err != nil {
		return nil, err
	}

	return &AnalyzeAvatarImportResponse{
		Batch: toAvatarImportBatch(batchRow),
		Items: items,
	}, nil
}

func (s *Service) GetAvatarImportBatch(ctx context.Context, batchID string) (*AnalyzeAvatarImportResponse, error) {
	batch, err := s.store.GetMediaImportBatchByID(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if batch == nil {
		return nil, errors.New("avatar import batch does not exist")
	}

	items, err := s.store.ListMediaImportBatchItems(ctx, batchID)
	if err != nil {
		return nil, err
	}

	viewItems, err := s.toAvatarImportBatchItems(ctx, items)
	if err != nil {
		return nil, err
	}

	return &AnalyzeAvatarImportResponse{
		Batch: toAvatarImportBatch(*batch),
		Items: viewItems,
	}, nil
}

func (s *Service) ConfirmAvatarImport(ctx context.Context, batchID string, request ConfirmAvatarImportRequest) (*ConfirmAvatarImportResponse, error) {
	if len(request.Items) == 0 {
		return nil, errors.New("at least one item must be confirmed")
	}
	replaceStrategy := strings.ToLower(strings.TrimSpace(request.ReplaceStrategy))
	if replaceStrategy == "" {
		replaceStrategy = avatarImportReplaceStrategyReplace
	}
	if replaceStrategy != avatarImportReplaceStrategyReplace && replaceStrategy != avatarImportReplaceStrategyKeep {
		return nil, errors.New("replaceStrategy must be either replace or keep")
	}

	batch, err := s.store.GetMediaImportBatchByID(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if batch == nil {
		return nil, errors.New("avatar import batch does not exist")
	}

	itemMap := make(map[string]string, len(request.Items))
	for _, item := range request.Items {
		if strings.TrimSpace(item.ItemID) == "" || strings.TrimSpace(item.StudentID) == "" {
			return nil, errors.New("itemId and studentId are required")
		}
		itemMap[item.ItemID] = item.StudentID
	}

	itemRows, err := s.store.ListMediaImportBatchItems(ctx, batchID)
	if err != nil {
		return nil, err
	}

	importedCount := 0
	now := time.Now().UTC()
	replacedStudents := make(map[string]struct{})

	for index := range itemRows {
		itemRow := itemRows[index]
		studentID, shouldImport := itemMap[itemRow.ID]
		if !shouldImport {
			continue
		}

		if itemRow.Status == avatarImportItemStatusImported {
			continue
		}

		if replaceStrategy == avatarImportReplaceStrategyKeep {
			existingAvatarCount, countErr := s.store.CountStudentMediaByType(ctx, studentID, mediaTypeAvatar)
			if countErr != nil {
				return nil, countErr
			}
			if existingAvatarCount > 0 {
				itemRows[index].ConfirmedStudentID = &studentID
				itemRows[index].Status = avatarImportItemStatusSkipped
				itemRows[index].ErrorMessage = stringPtr("Skipped because the student already has an avatar.")
				itemRows[index].UpdatedAt = now
				if err := s.store.UpdateMediaImportBatchItem(ctx, itemRows[index]); err != nil {
					return nil, err
				}
				continue
			}
		}

		if replaceStrategy == avatarImportReplaceStrategyReplace {
			if _, alreadyReplaced := replacedStudents[studentID]; !alreadyReplaced {
				existingAvatars, listErr := s.store.ListStudentMediaByType(ctx, studentID, mediaTypeAvatar)
				if listErr != nil {
					return nil, listErr
				}
				for _, existingAvatar := range existingAvatars {
					if err := s.DeleteAvatar(ctx, studentID, existingAvatar.ID); err != nil {
						return nil, err
					}
				}
				replacedStudents[studentID] = struct{}{}
			}
		}

		reader, err := s.storage.GetObject(ctx, itemRow.TempStorageKey)
		if err != nil {
			itemRows[index].Status = avatarImportItemStatusFailed
			itemRows[index].ErrorMessage = stringPtr(err.Error())
			itemRows[index].UpdatedAt = now
			_ = s.store.UpdateMediaImportBatchItem(ctx, itemRows[index])
			continue
		}

		data, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			itemRows[index].Status = avatarImportItemStatusFailed
			itemRows[index].ErrorMessage = stringPtr(readErr.Error())
			itemRows[index].UpdatedAt = now
			_ = s.store.UpdateMediaImportBatchItem(ctx, itemRows[index])
			continue
		}

		avatar, importErr := s.storeAvatarBytes(
			ctx,
			studentID,
			itemRow.OriginalFilename,
			itemRow.MimeType,
			data,
			avatarSourceBatchImport,
		)
		if importErr != nil {
			itemRows[index].Status = avatarImportItemStatusFailed
			itemRows[index].ErrorMessage = stringPtr(importErr.Error())
			itemRows[index].ConfirmedStudentID = &studentID
			itemRows[index].UpdatedAt = now
			_ = s.store.UpdateMediaImportBatchItem(ctx, itemRows[index])
			continue
		}

		itemRows[index].ConfirmedStudentID = &studentID
		itemRows[index].FinalMediaID = &avatar.ID
		itemRows[index].Status = avatarImportItemStatusImported
		itemRows[index].ErrorMessage = nil
		itemRows[index].UpdatedAt = now
		if err := s.store.UpdateMediaImportBatchItem(ctx, itemRows[index]); err != nil {
			return nil, err
		}

		_ = s.storage.DeleteObject(ctx, itemRow.TempStorageKey)
		importedCount += 1
	}

	updatedItems, err := s.store.ListMediaImportBatchItems(ctx, batchID)
	if err != nil {
		return nil, err
	}

	batch.Status = avatarImportBatchStatusCompleted
	batch.ImportedItems = countBatchItemsByStatus(updatedItems, avatarImportItemStatusImported)
	batch.FailedItems = countBatchItemsByStatus(updatedItems, avatarImportItemStatusFailed)
	batch.MatchedItems = countBatchItemsByStatus(updatedItems, avatarImportItemStatusMatched)
	batch.AmbiguousItems = countBatchItemsByStatus(updatedItems, avatarImportItemStatusAmbiguous)
	batch.UnmatchedItems = countBatchItemsByStatus(updatedItems, avatarImportItemStatusUnmatched)
	batch.ProcessedAt = &now
	batch.UpdatedAt = now
	if err := s.store.UpdateMediaImportBatch(ctx, *batch); err != nil {
		return nil, err
	}

	viewItems, err := s.toAvatarImportBatchItems(ctx, updatedItems)
	if err != nil {
		return nil, err
	}

	return &ConfirmAvatarImportResponse{
		Batch:         toAvatarImportBatch(*batch),
		ImportedCount: importedCount,
		Items:         viewItems,
	}, nil
}

func toAvatarImportBatch(row mediaImportBatchRow) AvatarImportBatch {
	return AvatarImportBatch{
		ID:             row.ID,
		Status:         row.Status,
		SourceType:     row.SourceType,
		OriginalFile:   row.OriginalFile,
		TotalItems:     row.TotalItems,
		MatchedItems:   row.MatchedItems,
		AmbiguousItems: row.AmbiguousItems,
		UnmatchedItems: row.UnmatchedItems,
		FailedItems:    row.FailedItems,
		ImportedItems:  row.ImportedItems,
		CreatedAt:      row.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      row.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ProcessedAt:    formatOptionalTime(row.ProcessedAt),
	}
}

func (s *Service) toAvatarImportBatchItems(ctx context.Context, rows []mediaImportBatchItemRow) ([]AvatarImportBatchItem, error) {
	items := make([]AvatarImportBatchItem, 0, len(rows))
	for _, row := range rows {
		previewURL := ""
		if row.TempStorageKey != "" {
			presignedURL, err := s.storage.PresignDownloadURL(ctx, row.TempStorageKey, s.presignExpiry)
			if err != nil {
				return nil, err
			}
			previewURL = presignedURL
		}

		items = append(items, AvatarImportBatchItem{
			ID:                 row.ID,
			BatchID:            row.BatchID,
			OriginalFilename:   row.OriginalFilename,
			MimeType:           row.MimeType,
			FileSize:           row.FileSize,
			GuessedStudentID:   row.GuessedStudentID,
			GuessedStudentName: row.GuessedStudentName,
			MatchMethod:        row.MatchMethod,
			MatchScore:         row.MatchScore,
			ConfirmedStudentID: row.ConfirmedStudentID,
			Status:             row.Status,
			ErrorMessage:       row.ErrorMessage,
			PreviewURL:         previewURL,
			FinalMediaID:       row.FinalMediaID,
		})
	}
	return items, nil
}

func buildAvatarMatchCandidates(rows []studentLookupRow) []avatarMatchCandidate {
	candidates := make([]avatarMatchCandidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, avatarMatchCandidate{
			ID:                 row.ID,
			StudentCode:        row.StudentCode,
			FullName:           row.FullName,
			NormalizedFullName: normalizeAvatarImportName(row.FullName),
		})
	}
	return candidates
}

type avatarMatchResult struct {
	status      string
	studentID   string
	studentName string
	method      string
	score       int
}

func matchAvatarImportStudent(filename string, candidates []avatarMatchCandidate) avatarMatchResult {
	stem := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	code := studentCodeImportPattern.FindString(strings.ToUpper(stem))
	if code != "" {
		for _, candidate := range candidates {
			if candidate.StudentCode != nil && strings.EqualFold(strings.TrimSpace(*candidate.StudentCode), code) {
				return avatarMatchResult{
					status:      avatarImportItemStatusMatched,
					studentID:   candidate.ID,
					studentName: candidate.FullName,
					method:      "student_code",
					score:       100,
				}
			}
		}
	}

	nameStem := normalizeAvatarImportName(stripImportStemNoise(stem))
	if nameStem == "" {
		return avatarMatchResult{status: avatarImportItemStatusUnmatched}
	}

	matches := make([]avatarMatchCandidate, 0)
	for _, candidate := range candidates {
		if candidate.NormalizedFullName == nameStem {
			matches = append(matches, candidate)
		}
	}

	switch len(matches) {
	case 0:
		return avatarMatchResult{status: avatarImportItemStatusUnmatched}
	case 1:
		return avatarMatchResult{
			status:      avatarImportItemStatusMatched,
			studentID:   matches[0].ID,
			studentName: matches[0].FullName,
			method:      "normalized_full_name",
			score:       90,
		}
	default:
		return avatarMatchResult{
			status: avatarImportItemStatusAmbiguous,
			method: "normalized_full_name",
			score:  60,
		}
	}
}

func buildAvatarImportTempStorageKey(batchID string, itemID string, filename string) string {
	return fmt.Sprintf("imports/avatar-batches/%s/%s/%s", batchID, itemID, sanitizeFilename(filename))
}

func newFailedImportItemRow(batchID string, itemID string, file AvatarImportUpload, now time.Time, message string) mediaImportBatchItemRow {
	return mediaImportBatchItemRow{
		ID:                itemID,
		BatchID:           batchID,
		OriginalFilename:  sanitizeFilename(file.Filename),
		TempStorageBucket: "",
		TempStorageKey:    "",
		MimeType:          file.ContentType,
		FileSize:          file.Size,
		MediaType:         mediaTypeAvatar,
		Status:            avatarImportItemStatusFailed,
		ErrorMessage:      stringPtr(message),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func normalizeAvatarImportName(value string) string {
	normalized := removeAccents(strings.ToLower(strings.TrimSpace(value)))
	normalized = strings.NewReplacer("_", " ", "-", " ").Replace(normalized)
	return strings.Join(strings.Fields(normalized), " ")
}

func stripImportStemNoise(stem string) string {
	cleaned := strings.TrimSpace(stem)
	cleaned = studentCodeImportPattern.ReplaceAllString(cleaned, "")
	cleaned = trailingTimestampPattern.ReplaceAllString(cleaned, "")
	cleaned = strings.Trim(cleaned, "-_ ")
	return cleaned
}

func countBatchItemsByStatus(items []mediaImportBatchItemRow, status string) int {
	count := 0
	for _, item := range items {
		if item.Status == status {
			count += 1
		}
	}
	return count
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return &trimmed
}

func intPtr(value int) *int {
	return &value
}
