package service

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/wwwrrrjjj/weiyu-go/config"
	"github.com/wwwrrrjjj/weiyu-go/models"
	"gorm.io/gorm"
)

var (
	ErrUsernameExists     = errors.New("用户名已存在")
	ErrInvalidUsername    = errors.New("用户名长度需在 3~50 个字符之间")
	ErrInvalidCredentials = errors.New("用户信息或密码错误")
	ErrUserNotFound       = errors.New("用户不存在")
)

// RegisterUser 注册用户。
// 用户名唯一性不依赖「先查再插」，直接插入并捕获唯一索引冲突，
// 这样并发注册同一用户名时只有一条能成功，不会产生脏数据。
func RegisterUser(ctx context.Context, username, password string) (*models.User, error) {
	username = strings.TrimSpace(username)
	if n := utf8.RuneCountInString(username); n < 3 || n > 50 {
		return nil, ErrInvalidUsername
	}

	user := models.User{
		Username: username,
	}
	if err := user.SetPassword(password); err != nil {
		return nil, err
	}

	if err := config.DB.WithContext(ctx).Create(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrUsernameExists
		}
		return nil, err
	}

	return &user, nil
}

// LoginUser 校验用户名密码。
// 用户不存在与密码错误返回同一个错误，避免通过错误信息枚举已注册用户名。
func LoginUser(ctx context.Context, username, password string) (*models.User, error) {
	var user models.User
	err := config.DB.WithContext(ctx).Where("username = ?", username).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if !user.CheckPassword(password) {
		return nil, ErrInvalidCredentials
	}

	return &user, nil
}

// GetUserByID 查询用户资料，供 /users/me 使用。
func GetUserByID(ctx context.Context, userID int64) (*models.User, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}

	var user models.User
	err := config.DB.WithContext(ctx).Where("id = ?", userID).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	return &user, nil
}
