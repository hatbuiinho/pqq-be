package media

type StudentMedia struct {
	ID               string  `json:"id"`
	StudentID        string  `json:"studentId"`
	MediaType        string  `json:"mediaType"`
	Title            *string `json:"title,omitempty"`
	Description      *string `json:"description,omitempty"`
	StorageBucket    string  `json:"storageBucket"`
	StorageKey       string  `json:"storageKey"`
	ThumbnailKey     *string `json:"thumbnailKey,omitempty"`
	OriginalFilename string  `json:"originalFilename"`
	MimeType         string  `json:"mimeType"`
	FileSize         int64   `json:"fileSize"`
	ChecksumSHA256   *string `json:"checksumSha256,omitempty"`
	IsPrimary        bool    `json:"isPrimary"`
	Source           string  `json:"source"`
	CapturedAt       *string `json:"capturedAt,omitempty"`
	UploadedAt       string  `json:"uploadedAt"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        string  `json:"updatedAt"`
	DeletedAt        *string `json:"deletedAt,omitempty"`
	DownloadURL      string  `json:"downloadUrl,omitempty"`
	ThumbnailURL     string  `json:"thumbnailUrl,omitempty"`
}

type UploadStudentAvatarResponse struct {
	Avatar StudentMedia `json:"avatar"`
}

type ListStudentAvatarsResponse struct {
	Items []StudentMedia `json:"items"`
}

type SetPrimaryStudentAvatarResponse struct {
	Avatar StudentMedia `json:"avatar"`
}

type AvatarImportBatch struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	SourceType     string  `json:"sourceType"`
	OriginalFile   *string `json:"originalFilename,omitempty"`
	TotalItems     int     `json:"totalItems"`
	MatchedItems   int     `json:"matchedItems"`
	AmbiguousItems int     `json:"ambiguousItems"`
	UnmatchedItems int     `json:"unmatchedItems"`
	FailedItems    int     `json:"failedItems"`
	ImportedItems  int     `json:"importedItems"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
	ProcessedAt    *string `json:"processedAt,omitempty"`
}

type AvatarImportBatchItem struct {
	ID                 string  `json:"id"`
	BatchID            string  `json:"batchId"`
	OriginalFilename   string  `json:"originalFilename"`
	MimeType           string  `json:"mimeType"`
	FileSize           int64   `json:"fileSize"`
	GuessedStudentID   *string `json:"guessedStudentId,omitempty"`
	GuessedStudentName *string `json:"guessedStudentName,omitempty"`
	MatchMethod        *string `json:"matchMethod,omitempty"`
	MatchScore         *int    `json:"matchScore,omitempty"`
	ConfirmedStudentID *string `json:"confirmedStudentId,omitempty"`
	Status             string  `json:"status"`
	ErrorMessage       *string `json:"errorMessage,omitempty"`
	PreviewURL         string  `json:"previewUrl,omitempty"`
	FinalMediaID       *string `json:"finalMediaId,omitempty"`
}

type AnalyzeAvatarImportResponse struct {
	Batch AvatarImportBatch       `json:"batch"`
	Items []AvatarImportBatchItem `json:"items"`
}

type ConfirmAvatarImportRequest struct {
	Items           []ConfirmAvatarImportItem `json:"items"`
	ReplaceStrategy string                    `json:"replaceStrategy,omitempty"`
}

type ConfirmAvatarImportItem struct {
	ItemID    string `json:"itemId"`
	StudentID string `json:"studentId"`
}

type ConfirmAvatarImportResponse struct {
	Batch         AvatarImportBatch       `json:"batch"`
	ImportedCount int                     `json:"importedCount"`
	Items         []AvatarImportBatchItem `json:"items"`
}
