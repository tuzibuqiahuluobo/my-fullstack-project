package main

import (
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ---------------------------------------------------------
// 1. 全局安全区（跨文件共享的变量）
// ---------------------------------------------------------
var db *gorm.DB                                // 全局数据库实例
var emailCodeMap = make(map[string]VerifyCode) // 验证码临时记事本

// ---------------------------------------------------------
// 2. 核心图纸区（所有的数据结构）
// ---------------------------------------------------------
type User struct {
	UID          uint   `json:"uid" gorm:"primaryKey;autoIncrement"`
	Username     string `json:"username" gorm:"unique"`
	PasswordHash string `json:"-"`
	Avatar       string `json:"avatar"`
	Email        string `json:"email" gorm:"unique"`
	Role         int    `json:"role"`
}

type Post struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type VerifyCode struct {
	Code      string
	ExpiresAt time.Time
}

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Code     string `json:"code"`
}

type UpdateRequest struct {
	UID      uint   `json:"uid"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	Password string `json:"password"`
}

type CreatePostRequest struct {
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	Content  string `json:"content"`
}

// ---------------------------------------------------------
// 3. 数据库初始化启动程序
// ---------------------------------------------------------
func initDB() {
	var err error
	// 注意：这里用的是 = 而不是 :=，是为了把连接实例赋给外面的全局 db 变量
	db, err = gorm.Open(sqlite.Open("data.db"), &gorm.Config{})
	if err != nil {
		fmt.Println("数据库连接失败:", err)
		return
	}

	// 同步表结构
	db.AutoMigrate(&User{}, &Post{})

	SuperAdminEmail := "2672172829@qq.com"
	SuperAdminPassword := "ASDasd5201314."

	var superAdmin User
	if result := db.Where("email = ?", SuperAdminEmail).First(&superAdmin); result.Error != nil {
		hash, _ := bcrypt.GenerateFromPassword([]byte(SuperAdminPassword), bcrypt.DefaultCost)
		db.Create(&User{
			Username:     "超级管理员",
			Email:        SuperAdminEmail,
			PasswordHash: string(hash),
			Role:         2,
			Avatar:       "https://api.dicebear.com/7.x/adventurer/svg?seed=Admin",
		})
		fmt.Println("👑 超级管理员账号已自动生成！")
	} else if superAdmin.Role != 2 {
		superAdmin.Role = 2
		db.Save(&superAdmin)
		fmt.Println("🛡️ 超级管理员权限已强制修复！")
	}
}
