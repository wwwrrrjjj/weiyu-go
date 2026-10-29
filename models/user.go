package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           int64     `gorm:"type:bigint;primaryKey;autoIncrement" json:"id"`
	Username     string    `gorm:"type:varchar(50);not null;uniqueIndex" json:"username"`
	PasswordHash string    `gorm:"type:varchar(255);not null" json:"-"`
	CreatedAt    time.Time `gorm:"type:datetime;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"type:datetime;autoUpdateTime" json:"updated_at"`
}

// SetPassword 将明文密码转换为 bcrypt 哈希
func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

// CheckPassword 验证密码
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword(
		[]byte(u.PasswordHash),
		[]byte(password),
	)
	return err == nil
}
