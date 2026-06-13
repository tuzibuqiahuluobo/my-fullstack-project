package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func publicUserPayload(user User) map[string]interface{} {
	return map[string]interface{}{
		"uid":      user.UID,
		"username": user.Username,
		"nickname": user.Nickname,
		"avatar":   user.Avatar,
		"role":     user.Role,
	}
}

func saveEmailCode(email string, code string) {
	emailCodeMu.Lock()
	defer emailCodeMu.Unlock()

	emailCodeMap[email] = VerifyCode{
		Code:      code,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
}

func generateVerifyCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()+100000), nil
}

// ---------------------------------------------------------
// 1. 注册接口
// ---------------------------------------------------------
func handleRegister(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式不对")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Code = strings.TrimSpace(req.Code)

	if req.Username == "" || req.Password == "" || req.Email == "" || req.Code == "" {
		writeError(w, http.StatusBadRequest, "用户名、密码、邮箱和验证码都不能为空")
		return
	}
	if len(req.Password) < 6 {
		writeError(w, http.StatusBadRequest, "密码至少需要 6 位")
		return
	}

	var existingUser User
	// 先用代码检查用户名或邮箱是否已存在，避免依赖数据库唯一索引。
	// 这样即使你的旧 data.db 里已经有重复邮箱，启动迁移也不会失败。
	if err := db.Where("username = ? OR email = ?", req.Username, req.Email).First(&existingUser).Error; err == nil {
		writeError(w, http.StatusBadRequest, "该用户名或邮箱已被注册")
		return
	}

	emailCodeMu.Lock()
	savedData, exists := emailCodeMap[req.Email]
	emailCodeMu.Unlock()
	if !exists {
		writeError(w, http.StatusUnauthorized, "请先获取验证码")
		return
	}
	if time.Now().After(savedData.ExpiresAt) {
		emailCodeMu.Lock()
		delete(emailCodeMap, req.Email)
		emailCodeMu.Unlock()
		writeError(w, http.StatusUnauthorized, "验证码已过期 (5分钟)，请重新发送")
		return
	}
	if savedData.Code != req.Code {
		writeError(w, http.StatusUnauthorized, "验证码错误")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密码加密失败")
		return
	}

	newUser := User{
		Username:          req.Username,
		PasswordHash:      string(hashedPassword),
		Email:             req.Email,
		Role:              0,
		UsernameUpdatedAt: time.Now(),
	}

	if result := db.Create(&newUser); result.Error != nil {
		writeError(w, http.StatusBadRequest, "该用户名或邮箱已被注册")
		return
	}

	newUser.Nickname = fmt.Sprintf("user_%d", newUser.UID)
	newUser.Avatar = fmt.Sprintf("https://api.dicebear.com/7.x/adventurer/svg?seed=user_%d", newUser.UID)
	if result := db.Save(&newUser); result.Error != nil {
		writeError(w, http.StatusInternalServerError, "默认资料保存失败")
		return
	}

	emailCodeMu.Lock()
	delete(emailCodeMap, req.Email)
	emailCodeMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"message": "注册成功！欢迎加入。"})
}

// ---------------------------------------------------------
// 2. 登录接口
// ---------------------------------------------------------
func handleLogin(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式不对")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "用户名和密码不能为空")
		return
	}

	var user User
	if result := db.Where("username = ?", req.Username).First(&user); result.Error != nil {
		writeError(w, http.StatusUnauthorized, "用户名不存在")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "密码错误")
		return
	}

	token, err := generateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "登录凭证生成失败")
		return
	}

	payload := publicUserPayload(user)
	payload["message"] = "登录成功！欢迎回来，" + user.Username
	payload["token"] = token
	writeJSON(w, http.StatusOK, payload)
}

// ---------------------------------------------------------
// 3. 修改资料接口
// ---------------------------------------------------------
func handleUpdate(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	loginUser, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式不对")
		return
	}

	var user User
	if result := db.First(&user, loginUser.UID); result.Error != nil {
		writeError(w, http.StatusNotFound, "找不到该用户")
		return
	}

	oldUsername := user.Username
	newNickname := strings.TrimSpace(req.Nickname)
	newAvatar := strings.TrimSpace(req.Avatar)
	newUsername := strings.TrimSpace(req.Username)
	newPassword := strings.TrimSpace(req.Password)
	currentPassword := strings.TrimSpace(req.CurrentPassword)
	usernameChanged := false

	if newNickname != "" {
		user.Nickname = newNickname
	}
	if newAvatar != "" {
		user.Avatar = newAvatar
	}

	if newUsername != "" && newUsername != user.Username {
		if currentPassword == "" {
			writeError(w, http.StatusForbidden, "修改登录账号必须输入当前密码进行安全验证")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
			writeError(w, http.StatusUnauthorized, "当前密码输入错误，无权更改账号")
			return
		}

		timeLimit := 180 * 24 * time.Hour
		durationSinceUpdate := time.Since(user.UsernameUpdatedAt)
		if durationSinceUpdate < timeLimit {
			remaining := timeLimit - durationSinceUpdate
			remainingDays := int(remaining.Hours() / 24)
			if remainingDays == 0 {
				remainingDays = 1
			}
			writeError(w, http.StatusForbidden, fmt.Sprintf("登录账号每 180 天仅可修改一次，距离下次解锁还剩 %d 天", remainingDays))
			return
		}

		var existingUser User
		if err := db.Where("username = ?", newUsername).First(&existingUser).Error; err == nil {
			writeError(w, http.StatusBadRequest, "该用户名已被他人占用，请换一个名字")
			return
		}

		user.Username = newUsername
		user.UsernameUpdatedAt = time.Now()
		usernameChanged = true
	}

	if newPassword != "" {
		if len(newPassword) < 6 {
			writeError(w, http.StatusBadRequest, "新密码至少需要 6 位")
			return
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "新密码加密失败")
			return
		}
		user.PasswordHash = string(hashedPassword)
	}

	if result := db.Save(&user); result.Error != nil {
		writeError(w, http.StatusInternalServerError, "保存失败，数据库写入错误")
		return
	}

	if usernameChanged {
		db.Model(&Post{}).Where("username = ?", oldUsername).Update("username", user.Username)
		db.Model(&Comment{}).Where("username = ?", oldUsername).Update("username", user.Username)
	}

	token, err := generateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "登录凭证刷新失败")
		return
	}

	payload := publicUserPayload(user)
	payload["message"] = "资料更新成功！"
	payload["token"] = token
	writeJSON(w, http.StatusOK, payload)
}

// ---------------------------------------------------------
// 4. 发送验证码接口
// ---------------------------------------------------------
func handleSendCode(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式错误")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.HasSuffix(email, "@qq.com") && !strings.HasSuffix(email, "@gmail.com") {
		writeError(w, http.StatusForbidden, "抱歉，目前仅支持 QQ 或 Gmail 邮箱注册")
		return
	}

	code, err := generateVerifyCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "验证码生成失败")
		return
	}

	senderEmail := getEnv("SMTP_USER", "")
	senderAuthCode := getEnv("SMTP_PASS", "")
	smtpHost := getEnv("SMTP_HOST", "smtp.qq.com")
	smtpPort := getEnv("SMTP_PORT", "587")

	if senderEmail == "" || senderAuthCode == "" {
		saveEmailCode(email, code)
		fmt.Println("开发模式验证码:", email, code)
		writeJSON(w, http.StatusOK, map[string]string{
			"message": "邮件服务未配置，开发验证码已输出到后端控制台",
		})
		return
	}

	message := []byte("From: <" + senderEmail + ">\r\n" +
		"To: " + email + "\r\n" +
		"Subject: 【开发者中心】您的注册验证码\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		"欢迎注册开发者中心！您的验证码是：" + code + "。请勿将此验证码泄露给他人。")

	auth := smtp.PlainAuth("", senderEmail, senderAuthCode, smtpHost)
	if err := smtp.SendMail(smtpHost+":"+smtpPort, auth, senderEmail, []string{email}, message); err != nil {
		fmt.Println("邮件发送失败:", err)
		writeError(w, http.StatusInternalServerError, "邮件发送失败，请检查服务器网络")
		return
	}

	saveEmailCode(email, code)
	writeJSON(w, http.StatusOK, map[string]string{"message": "验证码发送成功，请注意查收！"})
}

// ---------------------------------------------------------
// 5. 获取帖子列表接口
// ---------------------------------------------------------
func handleGetPosts(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	currentUser, hasLoginUser := currentUserFromRequest(r)

	var posts []Post
	if err := db.Order("created_at desc").Find(&posts).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "帖子读取失败")
		return
	}

	for i := 0; i < len(posts); i++ {
		var user User
		if err := db.Where("username = ?", posts[i].Username).First(&user).Error; err == nil {
			posts[i].Nickname = user.Nickname
			posts[i].Avatar = user.Avatar
		} else {
			posts[i].Nickname = "已注销用户"
			posts[i].Avatar = "https://api.dicebear.com/7.x/adventurer/svg?seed=deleted"
		}

		db.Where("post_id = ?", posts[i].ID).Order("created_at asc").Find(&posts[i].Comments)
		db.Model(&Favorite{}).Where("post_id = ?", posts[i].ID).Count(&posts[i].FavoriteCount)

		if hasLoginUser {
			var fav Favorite
			if err := db.Where("uid = ? AND post_id = ?", currentUser.UID, posts[i].ID).First(&fav).Error; err == nil {
				posts[i].IsFavorited = true
			}
		}
	}

	writeJSON(w, http.StatusOK, posts)
}

// ---------------------------------------------------------
// 6. 发布帖子接口
// ---------------------------------------------------------
func handleCreatePost(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式不对")
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeError(w, http.StatusBadRequest, "帖子内容不能为空")
		return
	}

	newPost := Post{
		Username:  user.Username,
		Nickname:  user.Nickname,
		Avatar:    user.Avatar,
		Content:   content,
		CreatedAt: time.Now(),
	}

	if result := db.Create(&newPost); result.Error != nil {
		writeError(w, http.StatusInternalServerError, "发帖失败，数据库错误")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "发布成功！"})
}

// ---------------------------------------------------------
// 7. 删除帖子接口
// ---------------------------------------------------------
func handleDeletePost(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req struct {
		PostID uint `json:"post_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式不对")
		return
	}

	var post Post
	if result := db.First(&post, req.PostID); result.Error != nil {
		writeError(w, http.StatusNotFound, "找不到该帖子，可能已被删除")
		return
	}

	if post.Username != user.Username && user.Role != 2 {
		writeError(w, http.StatusForbidden, "越权操作：您只能删除自己的帖子")
		return
	}

	db.Where("post_id = ?", post.ID).Delete(&Comment{})
	db.Where("post_id = ?", post.ID).Delete(&Favorite{})
	if result := db.Delete(&post); result.Error != nil {
		writeError(w, http.StatusInternalServerError, "删除失败，数据库出错")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "帖子已永久销毁"})
}

// ---------------------------------------------------------
// 8. 获取所有用户列表
// ---------------------------------------------------------
func handleGetUsers(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	var users []User
	if err := db.Order("uid asc").Find(&users).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "用户列表读取失败")
		return
	}

	writeJSON(w, http.StatusOK, users)
}

// ---------------------------------------------------------
// 9. 强制注销（删除）某个用户
// ---------------------------------------------------------
func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	admin, ok := requireAdmin(w, r)
	if !ok {
		return
	}

	var req struct {
		TargetUID uint `json:"target_uid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式不对")
		return
	}

	var target User
	if result := db.First(&target, req.TargetUID); result.Error != nil {
		writeError(w, http.StatusNotFound, "找不到该用户")
		return
	}
	if target.UID == admin.UID || target.Role == 2 {
		writeError(w, http.StatusForbidden, "不能删除超级管理员账号")
		return
	}

	db.Where("uid = ?", target.UID).Delete(&Favorite{})
	if result := db.Delete(&target); result.Error != nil {
		writeError(w, http.StatusInternalServerError, "注销失败，数据库错误")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "该用户已被强制注销"})
}

// ---------------------------------------------------------
// 10. 发表评论接口
// ---------------------------------------------------------
func handleCreateComment(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req struct {
		PostID  uint   `json:"post_id"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式不对")
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeError(w, http.StatusBadRequest, "评论内容不能为空")
		return
	}

	var post Post
	if result := db.First(&post, req.PostID); result.Error != nil {
		writeError(w, http.StatusNotFound, "找不到要评论的帖子")
		return
	}

	comment := Comment{
		PostID:    req.PostID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		Avatar:    user.Avatar,
		Content:   content,
		CreatedAt: time.Now(),
	}

	if err := db.Create(&comment).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "评论失败，数据库错误")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "评论成功！"})
}

// ---------------------------------------------------------
// 11. 切换收藏状态接口 (点一下收藏，再点一下取消)
// ---------------------------------------------------------
func handleToggleFavorite(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req struct {
		PostID uint `json:"post_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式不对")
		return
	}

	var post Post
	if result := db.First(&post, req.PostID); result.Error != nil {
		writeError(w, http.StatusNotFound, "找不到该帖子")
		return
	}

	var fav Favorite
	result := db.Where("uid = ? AND post_id = ?", user.UID, req.PostID).First(&fav)

	if result.Error == nil {
		db.Delete(&fav)
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": "已取消收藏", "is_favorited": false})
		return
	}

	newFav := Favorite{UID: user.UID, PostID: req.PostID}
	if err := db.Create(&newFav).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "收藏失败，数据库错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"message": "收藏成功", "is_favorited": true})
}

// ---------------------------------------------------------
// 12. 删除评论接口
// ---------------------------------------------------------
func handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req struct {
		CommentID uint `json:"comment_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式不对")
		return
	}

	var comment Comment
	if result := db.First(&comment, req.CommentID); result.Error != nil {
		writeError(w, http.StatusNotFound, "找不到该评论，可能已被删除")
		return
	}

	if comment.Username != user.Username && user.Role != 2 {
		writeError(w, http.StatusForbidden, "越权操作：您只能删除自己的评论")
		return
	}

	if result := db.Delete(&comment); result.Error != nil {
		writeError(w, http.StatusInternalServerError, "删除失败，数据库出错")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "评论已删除"})
}
