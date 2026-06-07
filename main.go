package main

import (
	"encoding/json"
	"fmt"
	"math/rand" // 【新增】用来生成随机数
	"net/http"
	"net/smtp" // 【新增】用来发邮件
	"strings"  // 【新增】用来处理字符串
	"time"     // 【新增】用来计算过期时间

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

	// 新增字段：邮箱和验证码
	Email string `json:"email"`
	Code  string `json:"code"` // 前端填写的邮箱验证码
}

// 专门接收前端传来的更新数据的结构体
type UpdateRequest struct {
	UID      uint   `json:"uid"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	Password string `json:"password"`
}

// 【新增】专门装验证码和过期时间的盒子
type VerifyCode struct {
	Code      string
	ExpiresAt time.Time // 记录这个验证码什么时间过期
}

// 【修改】升级版记事本，现在里面装的是 VerifyCode 盒子
var emailCodeMap = make(map[string]VerifyCode)

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
			Username:     "AdminUser",
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

		// 2. 【优化】核对邮箱验证码的有效性和时间
		savedData, exists := emailCodeMap[req.Email]
		if !exists {
			http.Error(w, `{"error": "请先获取验证码"}`, http.StatusUnauthorized)
			return
		}
		// 检查是否超过了 5 分钟
		if time.Now().After(savedData.ExpiresAt) {
			delete(emailCodeMap, req.Email) // 顺手把过期的垃圾清理掉
			http.Error(w, `{"error": "验证码已过期 (5分钟)，请重新发送"}`, http.StatusUnauthorized)
			return
		}
		// 检查数字是否匹配
		if savedData.Code != req.Code {
			http.Error(w, `{"error": "验证码错误"}`, http.StatusUnauthorized)
			return
		}

		// 3. 将明文密码进行 bcrypt 加密
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, `{"error": "密码加密失败"}`, http.StatusInternalServerError)
			return
		}

		// 4. 【更新】组装新用户，这次带上邮箱，并且默认 Role 为 0 (普通成员)
		newUser := User{
			Username:     req.Username,
			PasswordHash: string(hashedPassword),
			Email:        req.Email, // 存入前端传来的邮箱
			Role:         0,         // 0 代表普通用户
		}

		// 5. 保存到数据库 (如果用户名重复，这里会报错)
		result := db.Create(&newUser)
		if result.Error != nil {
			http.Error(w, `{"error": "该用户名已被注册或系统错误"}`, http.StatusBadRequest)
			return
		}

		// 6. 【新增安全机制】注册成功后，立刻销毁记事本里的验证码，防止被重复使用
		delete(emailCodeMap, req.Email)

		// 7. 告诉前端：注册成功
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

	// 【新增】发送邮箱验证码接口
	http.HandleFunc("/api/send-code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			return
		}

		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "数据格式错误"}`, http.StatusBadRequest)
			return
		}

		// 1. 拦截后缀：仅允许 qq.com 和 gmail.com
		email := strings.ToLower(req.Email)
		if !strings.HasSuffix(email, "@qq.com") && !strings.HasSuffix(email, "@gmail.com") {
			http.Error(w, `{"error": "抱歉，目前仅支持 QQ 或 Gmail 邮箱注册！"}`, http.StatusForbidden)
			return
		}

		// 2. 生成 6 位随机验证码
		code := fmt.Sprintf("%06d", rand.Intn(900000)+100000)

		// 3. 配置发件人信息
		senderEmail := "2672172829@qq.com"   // 替换成你的发件 QQ 邮箱
		senderAuthCode := "soxouqzypsdbdjee" // 替换成 SMTP 授权码
		smtpHost := "smtp.qq.com"
		smtpPort := "587"

		// 4. 拼装邮件内容
		message := []byte("From: <" + senderEmail + "\r\n" +
			"To: " + email + "\r\n" +
			"Subject: 【开发者中心】您的注册验证码\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
			"欢迎注册开发者中心！您的验证码是：" + code + "。请勿将此验证码泄露给他人。")

		// 5. 连接腾讯服务器并发射邮件
		auth := smtp.PlainAuth("", senderEmail, senderAuthCode, smtpHost)
		err := smtp.SendMail(smtpHost+":"+smtpPort, auth, senderEmail, []string{email}, message)
		if err != nil {
			fmt.Println("邮件发送失败:", err)
			http.Error(w, `{"error": "邮件发送失败，请检查服务器网络"}`, http.StatusInternalServerError)
			return
		}

		// 6. 发送成功后，把验证码和 5 分钟后的过期时间一起锁进保险箱
		emailCodeMap[email] = VerifyCode{
			Code:      code,
			ExpiresAt: time.Now().Add(5 * time.Minute), // 当前时间往后推 5 分钟
		}
		fmt.Println("✅ 成功向", email, "发送验证码:", code)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "验证码发送成功，请注意查收！",
		})
	})

	fmt.Println("🚀 服务器已启动！运行在 http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
