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

	// --- 强制加冕主管理员逻辑 ---
	SuperAdminEmail := "2672172829@qq.com"
	SuperAdminPassword := "ASDasd5201314."

	var superAdmin User
	if result := db.Where("email = ?", SuperAdminEmail).First(&superAdmin); result.Error != nil {
		hash, _ := bcrypt.GenerateFromPassword([]byte(SuperAdminPassword), bcrypt.DefaultCost)
		db.Create(&User{
			Username:     "最高指挥官",
			Email:        SuperAdminEmail,
			PasswordHash: string(hash),
			Role:         2,
			Avatar:       "https://api.dicebear.com/7.x/adventurer/svg?seed=Admin",
		})
		fmt.Println("👑 主管理员账号已自动生成！")
	} else if superAdmin.Role != 2 {
		superAdmin.Role = 2
		db.Save(&superAdmin)
		fmt.Println("🛡️ 主管理员权限已强制修复！")
	}

	// --- 社区初始动态逻辑 ---
	var count int64
	db.Model(&Post{}).Count(&count)
	if count == 0 {
		db.Create(&Post{
			Username:  "最高指挥官",
			Avatar:    "https://api.dicebear.com/7.x/adventurer/svg?seed=Admin",
			Content:   "分享一下我最近用 Godot 做战棋游戏的心得，HD-2D 画风真的太棒了！框架已经搭好，准备研究后续的章节剧情模块。",
			CreatedAt: time.Now().Add(-2 * time.Hour),
		})
		db.Create(&Post{
			Username:  "技术宅小明",
			Avatar:    "https://api.dicebear.com/7.x/adventurer/svg?seed=Ming",
			Content:   "有人知道用 Python 写类似《杀戮尖塔》的那种分层 DAG（有向无环图）地图生成，该用什么算法最优吗？卡在寻路逻辑这里了。",
			CreatedAt: time.Now().Add(-5 * time.Hour),
		})
		db.Create(&Post{
			Username:  "字幕烤肉Man",
			Avatar:    "https://api.dicebear.com/7.x/adventurer/svg?seed=Sub",
			Content:   "刚用 FFmpeg 把绝区零角色的 PV 字幕轴打完，爱芮的台词翻译和校对花了不少时间，准备压制导出了~",
			CreatedAt: time.Now().Add(-24 * time.Hour),
		})
		fmt.Println("📝 社区初始动态已加载！")
	}
}
