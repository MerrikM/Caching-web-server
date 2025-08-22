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
	Version          int        `db:"version" json:"version"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updated_at"`
	DeletedAt        *time.Time `db:"deleted_at" json:"deleted_at,omitempty"`
}

type DocumentShare struct {
	DocumentUUID   string    `db:"document_uuid" json:"document_uuid"`
	TargetUserUUID string    `db:"target_user_uuid" json:"target_user_uuid"`
	Permission     string    `db:"permission" json:"permission"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
}

type DocumentResponse struct {
	UUID         string `json:"uuid"`
	Title        string `json:"title"`
	Version      int    `json:"version"`
	PresignedURL string `json:"presigned_url"`
}
