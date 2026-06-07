package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	UID          uint   `gorm:"primaryKey;autoIncrement"`
	Username     string `gorm:"unique"`
	PasswordHash string
	Avatar       string
	Gender       string
	Age          int

	// 新增字段：邮箱和身份标识
	Email string `json:"email" gorm:"unique"` // 邮箱，必须唯一
	Role  int    `json:"role"`                // 身份标识：0=普通成员, 1=管理员, 2=主管理员(群主)
}

// 专门用来接收前端传来的临时数据的结构体
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// 专门接收前端传来的更新数据的结构体
type UpdateRequest struct {
	UID      uint   `json:"uid"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	Password string `json:"password"`
}

func main() {
	db, err := gorm.Open(sqlite.Open("data.db"), &gorm.Config{})
	if err != nil {
		fmt.Println("数据库连接失败:", err)
		return
	}
	db.AutoMigrate(&User{})

	// 【新增】主管理员强制加冕逻辑
	// 这里写死你的专属邮箱和初始密码。如果数据库里没有这个主管理员，系统会自动创建；
	// 如果别人试图在数据库里修改你的权限，系统每次重启都会强行把你恢复为 Role = 2
	SuperAdminEmail := "2672172829@qq.com" // 请替换成你真实的邮箱！
	SuperAdminPassword := "ASDasd5201314." // 你的初始最高权限密码

	var superAdmin User
	if result := db.Where("email = ?", SuperAdminEmail).First(&superAdmin); result.Error != nil {
		// 没找到主管理员，说明是第一次运行，直接创建
		hash, _ := bcrypt.GenerateFromPassword([]byte(SuperAdminPassword), bcrypt.DefaultCost)
		db.Create(&User{
			Username:     "最高指挥官",
			Email:        SuperAdminEmail,
			PasswordHash: string(hash),
			Role:         2, // 2 代表最高主管理员
			Avatar:       "https://api.dicebear.com/7.x/adventurer/svg?seed=Admin",
		})
		fmt.Println("👑 主管理员账号已自动生成！")
	} else if superAdmin.Role != 2 {
		// 如果找到了账号，但发现 Role 被人恶意改小了，强行改回 2
		superAdmin.Role = 2
		db.Save(&superAdmin)
		fmt.Println("🛡️ 主管理员权限已强制修复！")
	}

	// 【新增】注册接口
	http.HandleFunc("/api/register", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type") // 允许前端发送 JSON 数据

		// 处理预检请求 (浏览器的安全机制)
		if r.Method == "OPTIONS" {
			return
		}

		// 1. 拆开前端寄来的“信封”（解析 JSON）
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "数据格式不对"}`, http.StatusBadRequest)
			return
		}

		// 2. 将明文密码进行 bcrypt 加密
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, `{"error": "密码加密失败"}`, http.StatusInternalServerError)
			return
		}

		// 3. 按照我们的图纸，组装一个新用户
		newUser := User{
			Username:     req.Username,
			PasswordHash: string(hashedPassword),
		}

		// 4. 保存到数据库 (如果用户名重复，这里会报错)
		result := db.Create(&newUser)
		if result.Error != nil {
			http.Error(w, `{"error": "该用户名已被注册或系统错误"}`, http.StatusBadRequest)
			return
		}

		// 5. 告诉前端：注册成功
		fmt.Fprintf(w, `{"message": "注册成功！欢迎加入。"}`)
	})

	// 【新增】登录接口
	http.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			return
		}

		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "数据格式不对"}`, http.StatusBadRequest)
			return
		}

		// 1. 去数据库里找这个用户名
		var user User
		result := db.Where("username = ?", req.Username).First(&user)
		if result.Error != nil {
			// 找不到这个用户
			http.Error(w, `{"error": "用户名不存在"}`, http.StatusUnauthorized)
			return
		}

		// 2. 核心：比对密码（用 bcrypt 提供的方法，比对明文和数据库里的哈希值）
		err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
		if err != nil {
			// 密码碰对失败
			http.Error(w, `{"error": "密码错误"}`, http.StatusUnauthorized)
			return
		}

		// 登录成功！让 Go 自动把数据打包成标准的 JSON 并发送给前端
		w.Header().Set("Content-Type", "application/json") // 确保贴上 JSON 标签
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "登录成功！欢迎回来，" + user.Username,
			"uid":     user.UID,
			"avatar":  user.Avatar,
		})
	})

	// 【新增】修改资料接口
	http.HandleFunc("/api/update", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			return
		}

		var req UpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "数据格式不对"}`, http.StatusBadRequest)
			return
		}

		// 1. 查：根据前端传来的 UID，从数据库里把这个用户找出来
		var user User
		if result := db.First(&user, req.UID); result.Error != nil {
			http.Error(w, `{"error": "找不到该用户"}`, http.StatusNotFound)
			return
		}

		// 2. 改：如果前端传来了新名字或新头像，就替换掉旧的
		if req.Username != "" {
			user.Username = req.Username
		}
		if req.Avatar != "" {
			user.Avatar = req.Avatar
		}
		if req.Password != "" {
			hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err != nil {
				http.Error(w, `{"error": "密码加密失败"}`, http.StatusInternalServerError)
				return
			}
			user.PasswordHash = string(hashedPassword)
		}

		// 3. 存：把修改后的用户重新保存回数据库
		if result := db.Save(&user); result.Error != nil {
			// 如果新名字和别人撞车了，数据库的 unique 标签依然会在这里拦截
			http.Error(w, `{"error": "更新失败，该用户名可能已被别人占用"}`, http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, `{"message": "资料更新成功！"}`)
	})

	fmt.Println("🚀 服务器已启动！运行在 http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
