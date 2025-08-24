package model

import "time"

type Document struct {
	UUID             string     `db:"uuid" json:"uuid"`
	OwnerUUID        string     `db:"owner_uuid" json:"owner_uuid"`
	FilenameOriginal string     `db:"filename_original" json:"filename_original"`
	SizeBytes        int64      `db:"size_bytes" json:"size_bytes"`
	MimeType         string     `db:"mime_type" json:"mime_type"`
	Sha256           string     `db:"sha256" json:"sha256"`
	StoragePath      string     `db:"storage_path" json:"storage_path"`
	IsFile           bool       `db:"is_file" json:"file"`
	IsPublic         bool       `db:"is_public" json:"is_public"`
	AccessToken      string     `db:"access_token" json:"access_token"`
	GrantLogins      []string   `db:"grant_logins" json:"grant"`
	Version          int        `db:"version" json:"version"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updated_at"`
	DeletedAt        *time.Time `db:"deleted_at" json:"deleted_at,omitempty"`
}

type DocumentGrant struct {
	DocumentUUID   string    `db:"document_uuid" json:"document_uuid"`
	TargetUserUUID string    `db:"target_user_uuid" json:"target_user_uuid"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
}

type DocumentResponse struct {
	UUID         string    `json:"uuid"`
	Title        string    `json:"name"`
	PresignedURL string    `json:"presigned_url"`
	File         bool      `json:"file"`
	IsPublic     bool      `json:"is_public"`
	GrantLogins  []string  `json:"grant"`
	MimeType     string    `json:"mime_type"`
	CreatedAt    time.Time `json:"created_at"`
}

type GetDocumentResult struct {
	Document *Document
	GetURL   string // если IsFile=true, содержит pre-signed URL
}
