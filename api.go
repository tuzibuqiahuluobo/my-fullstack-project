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
		Username:          req.Username,
		PasswordHash:      string(hashedPassword),
		Email:             req.Email,
		Role:              0,
		UsernameUpdatedAt: time.Now(), // 初始注册视为一次修改，开始计算180天
	}

	result := db.Create(&newUser)
	if result.Error != nil {
		http.Error(w, `{"error": "该用户名或邮箱已被注册"}`, http.StatusBadRequest)
		return
	}

	// ✨【核心新增】生成默认昵称：user_ + 刚刚生成的自增 UID
	newUser.Nickname = fmt.Sprintf("user_%d", newUser.UID)
	// 如果用户没有设置头像，顺手给他一个根据UID生成的漂亮默认头像
	if newUser.Avatar == "" {
		newUser.Avatar = fmt.Sprintf("https://api.dicebear.com/7.x/adventurer/svg?seed=user_%d", newUser.UID)
	}
	db.Save(&newUser) // 将昵称和头像再次同步回数据库

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
		"message":  "登录成功！欢迎回来，" + user.Username,
		"uid":      user.UID,
		"avatar":   user.Avatar,
		"username": user.Username, // 把原用户名传回去备用
		"nickname": user.Nickname, // 【新增】把新的昵称发给前端
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
	// =========================================================
	// 基础资料 (允许随时修改)
	// =========================================================
	if req.Nickname != "" {
		user.Nickname = req.Nickname // 【新增】接收并修改昵称
	}
	if req.Avatar != "" {
		user.Avatar = req.Avatar
	}
	// =========================================================
	// 核心凭证 (不允许随时修改)
	// =========================================================
	// 1. 核对新老用户名
	if req.Username != "" && req.Username != user.Username {
		if req.CurrentPassword == "" {
			http.Error(w, `{"error": "拒绝执行：修改登录账号必须输入当前密码进行安全验证！"}`, http.StatusForbidden)
			return
		}
		// 用 bcrypt 比对当前输入的密码和数据库的哈希值
		err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword))
		if err != nil {
			http.Error(w, `{"error": "安全验证失败：当前密码输入错误，无权更改账号！"}`, http.StatusUnauthorized)
			return
		}
		timeLimit := 60 * 24 * time.Hour
		durationSinceUpdate := time.Since(user.UsernameUpdatedAt)

		if durationSinceUpdate < timeLimit {
			// 计算还剩多少天解锁
			remaining := timeLimit - durationSinceUpdate
			remainingDays := int(remaining.Hours() / 24)
			if remainingDays == 0 {
				remainingDays = 1 // 不足一天按一天算
			}
			http.Error(w, fmt.Sprintf(`{"error": "安全锁定中：登录账号每 180 天仅可修改一次！距离下次解锁还剩 %d 天"}`, remainingDays), http.StatusForbidden)
			return
		}

		var existingUser User
		if err := db.Where("username = ?", req.Username).First(&existingUser).Error; err == nil {
			http.Error(w, `{"error": "变更失败：该用户名已被他人占用，请换一个名字"}`, http.StatusBadRequest)
			return
		}
		user.Username = req.Username
		user.UsernameUpdatedAt = time.Now()
	}
	// 2. 如果前端传来了【新密码】
	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, `{"error": "新密码加密失败"}`, http.StatusInternalServerError)
			return
		}
		user.PasswordHash = string(hashedPassword)
	}
	// =========================================================
	// 数据同步归仓
	// =========================================================
	if result := db.Save(&user); result.Error != nil {
		http.Error(w, `{"error": "保存失败，数据库写入错误"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "资料更新成功！核心凭证已同步。",
	})
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

	// 2. 循环每一条帖子，去用户表动态抓取最新的昵称和头像
	for i := 0; i < len(posts); i++ {
		var user User
		// 根据帖子记录里的唯一 username，去用户表搜寻其当下的状态
		if err := db.Where("username = ?", posts[i].Username).First(&user).Error; err == nil {
			// 用用户表里最新的数据，覆盖掉帖子表里的旧快照
			posts[i].Nickname = user.Nickname
			posts[i].Avatar = user.Avatar
		} else {
			// 该用户如果被销户了，帖子会显示“已注销用户”
			posts[i].Nickname = "已注销用户"
			posts[i].Avatar = "https://api.dicebear.com/7.x/adventurer/svg?seed=deleted"
		}
	}

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
		Nickname:  req.Nickname, // ✨【新增这行】将昵称锁进数据库
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

	// 2. 权限核对：只有帖子的主人，或者“超级管理员”才有资格删除
	if post.Username != req.Username && req.Username != "超级管理员" {
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

// ---------------------------------------------------------
// 8. 获取所有用户列表
// ---------------------------------------------------------
func handleGetUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	// 安全检查：由于暂时没有做复杂的 JWT 令牌，通过 URL 参数或请求头核对身份
	adminName := r.URL.Query().Get("admin")
	if adminName != "超级管理员" {
		http.Error(w, `{"error": "越权访问：你不是超级管理员！"}`, http.StatusForbidden)
		return
	}

	var users []User
	// 查出所有用户（GORM 自动会忽略在模型里打上 json:"-" 的密码字段）
	db.Find(&users)

	json.NewEncoder(w).Encode(users)
}

// ---------------------------------------------------------
// 9. 强制注销（删除）某个用户
// ---------------------------------------------------------
func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	var req struct {
		AdminName string `json:"admin_name"`
		TargetUID uint   `json:"target_uid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "数据格式不对"}`, http.StatusBadRequest)
		return
	}

	// 核心校验
	if req.AdminName != "超级管理员" {
		http.Error(w, `{"error": "拒绝执行：只有超级管理员拥有终极裁决权！"}`, http.StatusForbidden)
		return
	}

	// 执行物理删除
	var user User
	if result := db.Delete(&user, req.TargetUID); result.Error != nil {
		http.Error(w, `{"error": "注销失败，数据库错误"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "该用户已被强制剥夺权限并注销账号",
	})
}
