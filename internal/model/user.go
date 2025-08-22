package model

import "time"

type User struct {
	UUID         string    `db:"uuid" json:"uuid"`
	Email        string    `db:"email" json:"email"`
	PasswordHash string    `db:"password_hash" json:"-"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}
