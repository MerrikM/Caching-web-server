package model

import "time"

type User struct {
	UUID         string    `db:"uuid" json:"uuid"`
	Login        string    `db:"login" json:"login"`
	PasswordHash string    `db:"password_hash" json:"-"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}
