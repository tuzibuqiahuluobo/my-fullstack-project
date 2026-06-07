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
}

// 专门用来接收前端传来的临时数据的结构体
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func main() {
	db, err := gorm.Open(sqlite.Open("data.db"), &gorm.Config{})
	if err != nil {
		fmt.Println("数据库连接失败:", err)
		return
	}
	db.AutoMigrate(&User{})

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

		// 3. 登录成功！返回成功信息和用户的 UID
		// 注意：为了方便拼装带有动态变量的 JSON，我们这里用了 fmt.Sprintf
		successMessage := fmt.Sprintf(`{"message": "登录成功！欢迎回来，%s", "uid": %d}`, user.Username, user.UID)
		fmt.Fprint(w, successMessage)
	})

	fmt.Println("🚀 服务器已启动！运行在 http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
