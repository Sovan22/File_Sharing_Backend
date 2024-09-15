package models

import (
	"gorm.io/gorm"
	"time"
)

type File struct {
	gorm.Model
	Name   string
	Size   int64
	URL    string
	UserID uint
	Type   string
	PublicUrl string 
	// `gorm:"column:public_url"`
	PublicUrlExpiry time.Time 
	// `gorm:"column:public_url_expiry"`
}