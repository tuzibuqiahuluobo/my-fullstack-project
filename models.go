package main

import (
	"fmt"
	"sync"
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
var emailCodeMu sync.Mutex                     // net/http 会并发处理请求，map 需要加锁保护

// ---------------------------------------------------------
// 2. 核心图纸区（所有的数据结构）
// ---------------------------------------------------------
type User struct {
	UID               uint      `json:"uid" gorm:"primaryKey;autoIncrement"`
	Username          string    `json:"username" gorm:"unique"`
	PasswordHash      string    `json:"-"`
	Avatar            string    `json:"avatar"`
	Email             string    `json:"email"`
	Role              int       `json:"role"`
	UsernameUpdatedAt time.Time `json:"username_updated_at"` // 【新增】记录上次修改用户名的时间
	Nickname          string    `json:"nickname"`            // 【新增】用户昵称
}

type Post struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"` // ✨【新增】用于前端展示的昵称
	Avatar    string    `json:"avatar"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`

	// 【虚拟字段】打上 gorm:"-" 标签，代表不存入帖子表，仅用于动态计算并传给前端
	Comments      []Comment `json:"comments" gorm:"-"`
	FavoriteCount int64     `json:"favorite_count" gorm:"-"`
	IsFavorited   bool      `json:"is_favorited" gorm:"-"`
}

type VerifyCode struct {
	Code      string
	ExpiresAt time.Time
}

type RegisterRequest struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	Email           string `json:"email"`
	Code            string `json:"code"`
	Nickname        string `json:"nickname"`         // 【新增】前端传来的新昵称
	NewUsername     string `json:"new_username"`     // 【新增】想要修改的新用户名
	CurrentPassword string `json:"current_password"` // 【新增】修改用户名时必须提供的当前密码验证
}

type UpdateRequest struct {
	UID             uint   `json:"uid"`
	Username        string `json:"username"`
	Avatar          string `json:"avatar"`
	Password        string `json:"password"`
	Nickname        string `json:"nickname"`         // 【新增】前端传来的新昵称
	NewUsername     string `json:"new_username"`     // 【新增】想要修改的新用户名
	CurrentPassword string `json:"current_password"` // 【新增】修改用户名时必须提供的当前密码验证
}

type CreatePostRequest struct {
	Username string `json:"username"`
	Nickname string `json:"nickname"` // 接收前端传来的昵称
	Avatar   string `json:"avatar"`
	Content  string `json:"content"`
}

// 评论表
type Comment struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	PostID    uint      `json:"post_id" gorm:"index"` // 建立索引提升查询性能
	Username  string    `json:"username"`             // 评论人账号
	Nickname  string    `json:"nickname"`             // 评论人昵称
	Avatar    string    `json:"avatar"`               // 评论人头像
	Content   string    `json:"content"`              // 评论内容
	CreatedAt time.Time `json:"created_at"`
}

// 收藏表 (联合主键，防止重复收藏)
type Favorite struct {
	UID    uint `gorm:"primaryKey"`
	PostID uint `gorm:"primaryKey"`
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
	db.AutoMigrate(&User{}, &Post{}, &Comment{}, &Favorite{})

	// 这两个默认值只方便本地学习时快速启动项目。
	// 如果要部署到公网，请务必通过环境变量设置自己的邮箱和强密码。
	SuperAdminEmail := getEnv("SUPER_ADMIN_EMAIL", "2672172829@qq.com")
	SuperAdminPassword := getEnv("SUPER_ADMIN_PASSWORD", "ASDasd5201314.")

	var superAdmin User
	if result := db.Where("email = ?", SuperAdminEmail).First(&superAdmin); result.Error != nil {
		hash, _ := bcrypt.GenerateFromPassword([]byte(SuperAdminPassword), bcrypt.DefaultCost)
		db.Create(&User{
			Username:     "超级管理员",
			Nickname:     "超级管理员", // 新增
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
