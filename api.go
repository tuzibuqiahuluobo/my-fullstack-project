package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ---------------------------------------------------------
// 1. 注册接口
// ---------------------------------------------------------
func handleRegister(w http.ResponseWriter, r *http.Request) {
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

	// 核对邮箱验证码
	savedData, exists := emailCodeMap[req.Email]
	if !exists {
		http.Error(w, `{"error": "请先获取验证码"}`, http.StatusUnauthorized)
		return
	}
	if time.Now().After(savedData.ExpiresAt) {
		delete(emailCodeMap, req.Email)
		http.Error(w, `{"error": "验证码已过期 (5分钟)，请重新发送"}`, http.StatusUnauthorized)
		return
	}
	if savedData.Code != req.Code {
		http.Error(w, `{"error": "验证码错误"}`, http.StatusUnauthorized)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error": "密码加密失败"}`, http.StatusInternalServerError)
		return
	}

	newUser := User{
		Username:     req.Username,
		PasswordHash: string(hashedPassword),
		Email:        req.Email,
		Role:         0,
	}

	result := db.Create(&newUser)
	if result.Error != nil {
		http.Error(w, `{"error": "该用户名或邮箱已被注册"}`, http.StatusBadRequest)
		return
	}

	delete(emailCodeMap, req.Email)
	fmt.Fprintf(w, `{"message": "注册成功！欢迎加入。"}`)
}

// ---------------------------------------------------------
// 2. 登录接口
// ---------------------------------------------------------
func handleLogin(w http.ResponseWriter, r *http.Request) {
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

	var user User
	result := db.Where("username = ?", req.Username).First(&user)
	if result.Error != nil {
		http.Error(w, `{"error": "用户名不存在"}`, http.StatusUnauthorized)
		return
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		http.Error(w, `{"error": "密码错误"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "登录成功！欢迎回来，" + user.Username,
		"uid":     user.UID,
		"avatar":  user.Avatar,
	})
}

// ---------------------------------------------------------
// 3. 修改资料接口
// ---------------------------------------------------------
func handleUpdate(w http.ResponseWriter, r *http.Request) {
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

	var user User
	if result := db.First(&user, req.UID); result.Error != nil {
		http.Error(w, `{"error": "找不到该用户"}`, http.StatusNotFound)
		return
	}

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

	if result := db.Save(&user); result.Error != nil {
		http.Error(w, `{"error": "更新失败，该用户名可能已被占用"}`, http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, `{"message": "资料更新成功！"}`)
}

// ---------------------------------------------------------
// 4. 发送验证码接口
// ---------------------------------------------------------
func handleSendCode(w http.ResponseWriter, r *http.Request) {
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

	email := strings.ToLower(req.Email)
	if !strings.HasSuffix(email, "@qq.com") && !strings.HasSuffix(email, "@gmail.com") {
		http.Error(w, `{"error": "抱歉，目前仅支持 QQ 或 Gmail 邮箱注册！"}`, http.StatusForbidden)
		return
	}

	code := fmt.Sprintf("%06d", rand.Intn(900000)+100000)

	// ⚠️ 这里需要替换成你自己的授权码！
	senderEmail := "2672172829@qq.com"
	senderAuthCode := "soxouqzypsdbdjee"
	smtpHost := "smtp.qq.com"
	smtpPort := "587"

	message := []byte("From: <" + senderEmail + ">\r\n" +
		"To: " + email + "\r\n" +
		"Subject: 【开发者中心】您的注册验证码\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		"欢迎注册开发者中心！您的验证码是：" + code + "。请勿将此验证码泄露给他人。")

	auth := smtp.PlainAuth("", senderEmail, senderAuthCode, smtpHost)
	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, senderEmail, []string{email}, message)
	if err != nil {
		fmt.Println("邮件发送失败:", err)
		http.Error(w, `{"error": "邮件发送失败，请检查服务器网络"}`, http.StatusInternalServerError)
		return
	}

	emailCodeMap[email] = VerifyCode{
		Code:      code,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	fmt.Println("✅ 成功向", email, "发送验证码:", code)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "验证码发送成功，请注意查收！",
	})
}

// ---------------------------------------------------------
// 5. 获取帖子列表接口
// ---------------------------------------------------------
func handleGetPosts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	var posts []Post
	db.Order("created_at desc").Find(&posts)

	json.NewEncoder(w).Encode(posts)
}

// ---------------------------------------------------------
// 6. 发布帖子接口
// ---------------------------------------------------------
func handleCreatePost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "数据格式不对"}`, http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, `{"error": "帖子内容不能为空"}`, http.StatusBadRequest)
		return
	}

	newPost := Post{
		Username:  req.Username,
		Avatar:    req.Avatar,
		Content:   req.Content,
		CreatedAt: time.Now(),
	}

	if result := db.Create(&newPost); result.Error != nil {
		http.Error(w, `{"error": "发帖失败，数据库错误"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "发布成功！",
	})
}

// ---------------------------------------------------------
// 7. 删除帖子接口
// ---------------------------------------------------------
func handleDeletePost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	// 临时定义一个结构体来接收前端传来的数据
	var req struct {
		PostID   uint   `json:"post_id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "数据格式不对"}`, http.StatusBadRequest)
		return
	}

	// 1. 去数据库里寻找这个帖子
	var post Post
	if result := db.First(&post, req.PostID); result.Error != nil {
		http.Error(w, `{"error": "找不到该帖子，可能已被删除"}`, http.StatusNotFound)
		return
	}

	// 2. 【核心安检】权限核对：只有帖子的主人，或者“最高指挥官”才有资格删除
	if post.Username != req.Username && req.Username != "最高指挥官" {
		http.Error(w, `{"error": "越权操作：您只能删除自己的帖子！"}`, http.StatusForbidden)
		return
	}

	// 3. 执行物理销毁
	if result := db.Delete(&post); result.Error != nil {
		http.Error(w, `{"error": "删除失败，数据库出错"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "帖子已永久销毁",
	})
}
