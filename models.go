package main

import (
	"fmt"
	"os"
	"strings"
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
	Signature         string    `json:"signature"`           // 【新增】个性签名，最多 50 个字
}

type Post struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"` // ✨【新增】用于前端展示的昵称
	Avatar    string    `json:"avatar"`
	TopicID   uint      `json:"topic_id" gorm:"index"` // 新增：帖子用 topic_id 绑定话题，查询某个社区时就能直接按这个字段过滤。
	TagsRaw   string    `json:"-" gorm:"column:tags"`  // 新增：标签数量可变，用 JSON 字符串存入 SQLite，初学阶段比新建多对多表更容易理解。
	Title     string    `json:"title"`                 // 新增：帖子标题可以为空；为空时前端直接隐藏标题行。
	Content   string    `json:"content"`
	Image     string    `json:"image"`                  // 兼容旧数据：旧版本只保存单张图片，新版本会继续返回第一张图。
	ImagesRaw string    `json:"-" gorm:"column:images"` // 新增：多图用 JSON 字符串存入 SQLite，避免额外建表增加初学理解成本。
	CreatedAt time.Time `json:"created_at"`

	// 【虚拟字段】打上 gorm:"-" 标签，代表不存入帖子表，仅用于动态计算并传给前端
	Comments      []Comment `json:"comments" gorm:"-"`
	FavoriteCount int64     `json:"favorite_count" gorm:"-"`
	IsFavorited   bool      `json:"is_favorited" gorm:"-"`
	TopicName     string    `json:"topic_name" gorm:"-"`   // 新增：返回给前端展示的话题名，不重复存入帖子表。
	TopicStatus   string    `json:"topic_status" gorm:"-"` // 新增：详情页和后台可以知道该话题当前是否已停用。
	Tags          []string  `json:"tags" gorm:"-"`         // 新增：返回给前端展示的标签数组，不直接作为数据库字段。
	Signature     string    `json:"signature" gorm:"-"`    // 【新增】帖子表里不单独存签名，返回时读取作者当前签名
	Images        []string  `json:"images" gorm:"-"`       // 新增：返回给前端的多图数组，不直接作为数据库字段。
}

const (
	TopicStatusPending  = "pending"
	TopicStatusApproved = "approved"
	TopicStatusDisabled = "disabled"
	TopicStatusRejected = "rejected"
	DefaultTopicName    = "综合社区"
)

type Topic struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"uniqueIndex;size:40"`
	Description string    `json:"description" gorm:"size:200"`
	SortOrder   int       `json:"sort_order" gorm:"index"`
	Status      string    `json:"status" gorm:"size:20;index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	PostCount int64 `json:"post_count" gorm:"-"` // 新增：只给后台/前台展示数量，不需要单独存数据库。
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
	UID             uint    `json:"uid"`
	Username        string  `json:"username"`
	Avatar          string  `json:"avatar"`
	Password        string  `json:"password"`
	Nickname        string  `json:"nickname"`         // 【新增】前端传来的新昵称
	Signature       *string `json:"signature"`        // 【新增】指针可以区分“没传”和“传了空字符串清空签名”
	NewUsername     string  `json:"new_username"`     // 【新增】想要修改的新用户名
	CurrentPassword string  `json:"current_password"` // 【新增】修改用户名时必须提供的当前密码验证
}

type CreatePostRequest struct {
	Username string   `json:"username"`
	TopicID  uint     `json:"topic_id"` // 新增：前端发布时必须告诉后端发到哪个话题社区。
	Nickname string   `json:"nickname"` // 接收前端传来的昵称
	Avatar   string   `json:"avatar"`
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags"`   // 新增：用户自定义标签，最多 5 个，和话题互相独立。
	Image    string   `json:"image"`  // 兼容旧前端单图字段
	Images   []string `json:"images"` // 新增：发帖时可选的多图数组，最多 9 张。
}

type UpdatePostRequest struct {
	PostID  uint     `json:"post_id"`
	TopicID uint     `json:"topic_id"` // 新增：编辑帖子时也允许把帖子移动到另一个已通过话题。
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`   // 新增：编辑帖子时提交完整标签列表。
	Image   string   `json:"image"`  // 兼容旧前端单图字段
	Images  []string `json:"images"` // 新增：编辑帖子时提交完整图片列表。
}

// 评论表
type Comment struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	PostID    uint      `json:"post_id" gorm:"index"`   // 建立索引提升查询性能
	ParentID  uint      `json:"parent_id" gorm:"index"` // 新增：回复某条评论时记录父评论 ID，0 代表直接评论帖子。
	Username  string    `json:"username"`               // 评论人账号
	Nickname  string    `json:"nickname"`               // 评论人昵称
	Avatar    string    `json:"avatar"`                 // 评论人头像
	Content   string    `json:"content"`                // 评论内容
	ImagesRaw string    `json:"-" gorm:"column:images"` // 新增：评论图片也用 JSON 字符串保存，和帖子多图规则保持一致。
	CreatedAt time.Time `json:"created_at"`

	ReplyToUsername string   `json:"reply_to_username" gorm:"-"` // 新增：返回给前端展示“回复了谁”，不额外存数据库。
	ReplyToNickname string   `json:"reply_to_nickname" gorm:"-"` // 新增：昵称可能变化，读取时根据父评论实时补上。
	Images          []string `json:"images" gorm:"-"`            // 新增：返回给前端展示的评论图片数组。
}

type CreateCommentRequest struct {
	PostID   uint     `json:"post_id"`
	ParentID uint     `json:"parent_id"` // 新增：为 0 时是普通评论，不为 0 时就是回复某条评论。
	Content  string   `json:"content"`
	Image    string   `json:"image"`  // 兼容以后可能出现的单图字段
	Images   []string `json:"images"` // 新增：评论区复用帖子图片系统，最多 9 张。
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
	db.AutoMigrate(&User{}, &Topic{}, &Post{}, &Comment{}, &Favorite{})
	seedDefaultTopicsAndMigratePosts()

	// 这三个默认值只方便本地学习时快速启动项目。
	// 如果要部署到公网，请务必通过环境变量设置自己的账号、邮箱和强密码。
	SuperAdminUsername := getEnv("SUPER_ADMIN_USERNAME", "superadmin")
	SuperAdminEmail := getEnv("SUPER_ADMIN_EMAIL", "2672172829@qq.com")
	SuperAdminPassword := getEnv("SUPER_ADMIN_PASSWORD", "ASDasd5201314.")
	hasConfiguredAdminUsername := strings.TrimSpace(os.Getenv("SUPER_ADMIN_USERNAME")) != ""
	hasConfiguredAdminEmail := strings.TrimSpace(os.Getenv("SUPER_ADMIN_EMAIL")) != ""
	hasConfiguredAdminPassword := strings.TrimSpace(os.Getenv("SUPER_ADMIN_PASSWORD")) != ""

	ensureSuperAdminAccount(SuperAdminUsername, SuperAdminEmail, SuperAdminPassword, hasConfiguredAdminUsername, hasConfiguredAdminEmail, hasConfiguredAdminPassword)
}

func ensureSuperAdminAccount(superAdminUsername string, superAdminEmail string, superAdminPassword string, hasConfiguredUsername bool, hasConfiguredEmail bool, hasConfiguredPassword bool) {
	// 新增：把超级管理员同步逻辑抽出来，测试和真实启动都能复用同一套规则。
	// 这样之后再改 .env 生效方式时，不容易出现“测试通过但启动逻辑忘记改”的问题。
	var superAdmin User
	hasSuperAdmin := db.Where("role = ?", 2).First(&superAdmin).Error == nil

	if hasConfiguredUsername {
		var configuredUser User
		if err := db.Where("username = ?", superAdminUsername).First(&configuredUser).Error; err == nil {
			// 新增：如果 .env 里的管理员账号已经是普通用户，就直接接管这条记录。
			// 这样你删掉旧管理员后，不会因为同名普通用户占着 username 导致新管理员创建失败。
			if hasSuperAdmin && configuredUser.UID != superAdmin.UID {
				db.Model(&User{}).Where("uid = ?", superAdmin.UID).Update("role", 0)
			}
			superAdmin = configuredUser
			hasSuperAdmin = true
		}
	}

	if !hasSuperAdmin {
		hash, _ := bcrypt.GenerateFromPassword([]byte(superAdminPassword), bcrypt.DefaultCost)
		admin := User{
			Username:     superAdminUsername,
			Nickname:     superAdminUsername, // 新增
			Email:        superAdminEmail,
			PasswordHash: string(hash),
			Role:         2,
			Avatar:       "https://api.dicebear.com/7.x/adventurer/svg?seed=Admin",
		}
		if err := db.Create(&admin).Error; err != nil {
			fmt.Println("❌ 超级管理员创建失败，请检查 SUPER_ADMIN_USERNAME 或 SUPER_ADMIN_EMAIL 是否被占用:", err)
			return
		}
		fmt.Println("👑 超级管理员账号已自动生成！")
	} else {
		// 已经存在超级管理员时，默认不覆盖后台手动改过的信息。
		// 但如果 .env 里明确写了 SUPER_ADMIN_*，就同步这些配置，方便部署后直接用自己设置的账号密码登录。
		superAdmin.Role = 2
		if hasConfiguredUsername {
			superAdmin.Username = superAdminUsername
			superAdmin.Nickname = superAdminUsername
		}
		if hasConfiguredEmail {
			superAdmin.Email = superAdminEmail
		}
		if hasConfiguredPassword {
			hash, _ := bcrypt.GenerateFromPassword([]byte(superAdminPassword), bcrypt.DefaultCost)
			superAdmin.PasswordHash = string(hash)
		}
		if superAdmin.Avatar == "" {
			superAdmin.Avatar = "https://api.dicebear.com/7.x/adventurer/svg?seed=Admin"
		}
		if err := db.Save(&superAdmin).Error; err != nil {
			fmt.Println("❌ 超级管理员同步失败，请检查 .env 是否和已有用户账号/邮箱冲突:", err)
			return
		}
		fmt.Println("🛡️ 超级管理员权限已确认！")
	}
}

func seedDefaultTopicsAndMigratePosts() {
	defaultTopics := []Topic{
		{Name: DefaultTopicName, Description: "所有旧帖子和默认内容都会先放在这里。", SortOrder: 1, Status: TopicStatusApproved},
		{Name: "日常分享", Description: "记录生活里的小事、心情和灵感。", SortOrder: 2, Status: TopicStatusApproved},
		{Name: "学习交流", Description: "适合讨论学习笔记、技术问题和成长经验。", SortOrder: 3, Status: TopicStatusApproved},
		{Name: "作品展示", Description: "展示绘画、文章、项目和其他创意作品。", SortOrder: 4, Status: TopicStatusApproved},
		{Name: "问题求助", Description: "遇到问题时可以在这里向大家求助。", SortOrder: 5, Status: TopicStatusApproved},
		{Name: "游戏交流", Description: "分享游戏体验、攻略和网页小游戏想法。", SortOrder: 6, Status: TopicStatusApproved},
	}

	for _, topic := range defaultTopics {
		var existing Topic
		if err := db.Where("name = ?", topic.Name).First(&existing).Error; err != nil {
			// 新增：这里先查再创建，是为了避免每次后端启动都重复插入同名话题。
			db.Create(&topic)
		}
	}

	var general Topic
	if err := db.Where("name = ?", DefaultTopicName).First(&general).Error; err == nil {
		// 新增：老帖子以前没有 topic_id，统一迁到“综合社区”，升级后用户还能继续看到旧内容。
		db.Model(&Post{}).Where("topic_id = ? OR topic_id IS NULL", 0).Update("topic_id", general.ID)
	}
}
