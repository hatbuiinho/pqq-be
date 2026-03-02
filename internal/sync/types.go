package sync

import "encoding/json"

type EntityName string
type Operation string

const (
	EntityClubs     EntityName = "clubs"
	EntityBeltRanks EntityName = "belt_ranks"
	EntityStudents  EntityName = "students"

	OperationUpsert Operation = "upsert"
	OperationDelete Operation = "delete"
)

type PushRequest struct {
	DeviceID  string         `json:"deviceId"`
	Mutations []SyncMutation `json:"mutations"`
}

type SyncMutation struct {
	MutationID       string          `json:"mutationId"`
	EntityName       EntityName      `json:"entityName"`
	Operation        Operation       `json:"operation"`
	RecordID         string          `json:"recordId"`
	Record           json.RawMessage `json:"record"`
	ClientModifiedAt string          `json:"clientModifiedAt"`
}

type AppliedRecord struct {
	EntityName       EntityName      `json:"entityName"`
	Record           json.RawMessage `json:"record"`
	ServerModifiedAt string          `json:"serverModifiedAt"`
}

type Conflict struct {
	MutationID   string          `json:"mutationId"`
	EntityName   EntityName      `json:"entityName"`
	RecordID     string          `json:"recordId"`
	Reason       string          `json:"reason"`
	Message      string          `json:"message"`
	ServerRecord json.RawMessage `json:"serverRecord,omitempty"`
}

type PushResponse struct {
	ServerTime string          `json:"serverTime"`
	Applied    []AppliedRecord `json:"applied"`
	Conflicts  []Conflict      `json:"conflicts"`
}

type PullRequest struct {
	DeviceID string
	Since    string
	Limit    int
}

type PullChange struct {
	EntityName       EntityName      `json:"entityName"`
	Record           json.RawMessage `json:"record"`
	ServerModifiedAt string          `json:"serverModifiedAt"`
}

type PullResponse struct {
	ServerTime string       `json:"serverTime"`
	NextSince  string       `json:"nextSince"`
	HasMore    bool         `json:"hasMore"`
	Changes    []PullChange `json:"changes"`
}

type RebaseResponse struct {
	ServerTime string           `json:"serverTime"`
	Clubs      []ClubRecord     `json:"clubs"`
	BeltRanks  []BeltRankRecord `json:"beltRanks"`
	Students   []StudentRecord  `json:"students"`
}

type ClubImportRowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type ImportClubsResponse struct {
	ImportedCount int                  `json:"importedCount"`
	Errors        []ClubImportRowError `json:"errors"`
}

type BeltRankImportRowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type ImportBeltRanksResponse struct {
	ImportedCount int                      `json:"importedCount"`
	Errors        []BeltRankImportRowError `json:"errors"`
}

type RealtimeEvent struct {
	Type         string       `json:"type"`
	ConnectionID string       `json:"connectionId,omitempty"`
	ServerTime   string       `json:"serverTime"`
	EntityNames  []EntityName `json:"entityNames,omitempty"`
	RecordIDs    []string     `json:"recordIds,omitempty"`
}

type BaseRecord struct {
	ID             string  `json:"id"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
	LastModifiedAt string  `json:"lastModifiedAt"`
	SyncStatus     string  `json:"syncStatus"`
	DeletedAt      *string `json:"deletedAt,omitempty"`
}

type ClubRecord struct {
	BaseRecord
	Code     *string `json:"code,omitempty"`
	Name     string  `json:"name"`
	Phone    *string `json:"phone,omitempty"`
	Email    *string `json:"email,omitempty"`
	Address  *string `json:"address,omitempty"`
	Notes    *string `json:"notes,omitempty"`
	IsActive bool    `json:"isActive"`
}

type BeltRankRecord struct {
	BaseRecord
	Name        string  `json:"name"`
	Order       int     `json:"order"`
	Description *string `json:"description,omitempty"`
	IsActive    bool    `json:"isActive"`
}

type StudentRecord struct {
	BaseRecord
	StudentCode *string `json:"studentCode,omitempty"`
	FullName    string  `json:"fullName"`
	DateOfBirth *string `json:"dateOfBirth,omitempty"`
	Gender      *string `json:"gender,omitempty"`
	Phone       *string `json:"phone,omitempty"`
	Email       *string `json:"email,omitempty"`
	Address     *string `json:"address,omitempty"`
	ClubID      string  `json:"clubId"`
	BeltRankID  string  `json:"beltRankId"`
	JoinedAt    *string `json:"joinedAt,omitempty"`
	Status      string  `json:"status"`
	Notes       *string `json:"notes,omitempty"`
}

type StoredRecord struct {
	ChangeID         int64
	EntityName       EntityName
	RecordID         string
	Payload          []byte
	DeletedAt        *string
	LastModifiedAt   string
	ServerModifiedAt string
}
