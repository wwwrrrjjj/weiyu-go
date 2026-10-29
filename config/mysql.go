package config

import (
	"context"
	"fmt"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

// InitMySQL 初始化数据库连接池，并完成表结构自动迁移。
func InitMySQL(models ...interface{}) error {
	// 1. 读取数据库连接信息
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		return fmt.Errorf("MYSQL_DSN 未配置")
	}

	// 2. 创建 GORM 数据库对象
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		TranslateError: true, // 将驱动错误翻译为 gorm.ErrDuplicatedKey 等语义化错误
	})
	if err != nil {
		return err
	}

	// 3. 获取底层连接池并设置参数：
	//    MaxOpenConns 防止瞬时流量打满数据库连接数，
	//    ConnMaxLifetime 避免连接被数据库/中间件单方面断开后继续复用。
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 4. 检查数据库连接
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return err
	}

	// 5. 自动迁移数据库表
	if err := db.AutoMigrate(models...); err != nil {
		_ = sqlDB.Close()
		return err
	}

	DB = db
	return nil
}

// PingMySQL 供健康检查使用，探测底层连接是否可用。
func PingMySQL(ctx context.Context) error {
	if DB == nil {
		return fmt.Errorf("mysql 未初始化")
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func CloseMySQL() error {
	if DB == nil {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}
