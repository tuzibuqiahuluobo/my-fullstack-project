package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func publicUserPayload(user User) map[string]interface{} {
	return map[string]interface{}{
		"uid":       user.UID,
		"username":  user.Username,
		"nickname":  user.Nickname,
		"signature": user.Signature,
		"avatar":    user.Avatar,
		"role":      user.Role,
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

func validateSupportedEmail(email string) bool {
	// 目前邮箱验证码只开放 QQ 和 Gmail，前后端都限制一次，避免用户填了无法发送的邮箱。
	return strings.HasSuffix(email, "@qq.com") || strings.HasSuffix(email, "@gmail.com")
}

func sendVerifyCodeToEmail(email string, subject string, bodyPrefix string) (string, error) {
	// 统一生成验证码，注册、找回账号、重置密码都用同一套规则，后续维护会更简单。
	code, err := generateVerifyCode()
	if err != nil {
		return "", err
	}

	// SMTP_PASS 是邮箱授权码，不能写死在代码里；本地没配置时会退回控制台验证码，方便开发调试。
	senderEmail := getEnv("SMTP_USER", "2672172829@qq.com")
	senderAuthCode := getEnv("SMTP_PASS", "")
	smtpHost := getEnv("SMTP_HOST", "smtp.qq.com")
	smtpPort := getEnv("SMTP_PORT", "587")

	if senderEmail == "" || senderAuthCode == "" {
		saveEmailCode(email, code)
		fmt.Println("开发模式验证码:", email, code)
		return "邮件服务未配置，开发验证码已输出到后端控制台", nil
	}

	// 邮件正文集中在这里组装，注册、找回账号、重置密码都可以复用同一套发送逻辑。
	message := []byte("From: <" + senderEmail + ">\r\n" +
		"To: " + email + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		bodyPrefix + "您的验证码是：" + code + "。验证码 5 分钟内有效，请勿泄露给他人。")

	auth := smtp.PlainAuth("", senderEmail, senderAuthCode, smtpHost)
	if err := smtp.SendMail(smtpHost+":"+smtpPort, auth, senderEmail, []string{email}, message); err != nil {
		return "", err
	}

	saveEmailCode(email, code)
	return "验证码发送成功，请注意查收！", nil
}

func verifyEmailCode(email string, code string) (bool, string) {
	// map 是共享内存，读写时加锁可以避免多个请求同时操作造成数据错乱。
	emailCodeMu.Lock()
	savedData, exists := emailCodeMap[email]
	emailCodeMu.Unlock()
	if !exists {
		return false, "请先获取验证码"
	}
	if time.Now().After(savedData.ExpiresAt) {
		emailCodeMu.Lock()
		delete(emailCodeMap, email)
		emailCodeMu.Unlock()
		return false, "验证码已过期 (5分钟)，请重新发送"
	}
	if savedData.Code != code {
		return false, "验证码错误"
	}
	return true, ""
}

func clearEmailCode(email string) {
	// 验证码使用成功后立刻删除，避免同一个验证码被重复使用。
	emailCodeMu.Lock()
	delete(emailCodeMap, email)
	emailCodeMu.Unlock()
}

func enrichPostForResponse(post *Post, currentUser User, hasLoginUser bool) {
	var author User
	if err := db.Where("username = ?", post.Username).First(&author).Error; err == nil {
		post.Nickname = author.Nickname
		post.Avatar = author.Avatar
		post.Signature = author.Signature
	} else {
		post.Nickname = "已注销用户"
		post.Avatar = "https://api.dicebear.com/7.x/adventurer/svg?seed=deleted"
		post.Signature = ""
	}

	// 帖子详情、社区列表、我的收藏都需要这些展示数据，集中到这里避免三处写重复逻辑。
	db.Where("post_id = ?", post.ID).Order("created_at asc").Find(&post.Comments)
	db.Model(&Favorite{}).Where("post_id = ?", post.ID).Count(&post.FavoriteCount)
	post.IsFavorited = false
	if hasLoginUser {
		var fav Favorite
		if err := db.Where("uid = ? AND post_id = ?", currentUser.UID, post.ID).First(&fav).Error; err == nil {
			post.IsFavorited = true
		}
	}
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

	if ok, message := verifyEmailCode(req.Email, req.Code); !ok {
		writeError(w, http.StatusUnauthorized, message)
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

	clearEmailCode(req.Email)

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
// 2.1 获取当前登录用户资料接口
// ---------------------------------------------------------
func handleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	// Dashboard 不能只依赖 localStorage 里的旧资料，所以提供一个接口读取数据库里的最新昵称、头像和个性签名。
	writeJSON(w, http.StatusOK, publicUserPayload(user))
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
	if req.Signature != nil {
		newSignature := strings.TrimSpace(*req.Signature)
		if len([]rune(newSignature)) > 50 {
			writeError(w, http.StatusBadRequest, "个性签名最多 50 个字")
			return
		}
		// 个性签名允许清空，所以用指针判断前端是否真的提交了这个字段。
		user.Signature = newSignature
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
	if !validateSupportedEmail(email) {
		writeError(w, http.StatusForbidden, "抱歉，目前仅支持 QQ 或 Gmail 邮箱注册")
		return
	}

	message, err := sendVerifyCodeToEmail(email, "【SunShine】您的验证码", "欢迎使用 SunShine！")
	if err != nil {
		fmt.Println("邮件发送失败:", err)
		writeError(w, http.StatusInternalServerError, "邮件发送失败，请检查服务器网络")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": message})
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
		enrichPostForResponse(&posts[i], currentUser, hasLoginUser)
	}

	writeJSON(w, http.StatusOK, posts)
}

// ---------------------------------------------------------
// 6. 获取单条帖子详情
// ---------------------------------------------------------
func handleGetPostDetail(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	postID, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("id")))
	if err != nil || postID <= 0 {
		writeError(w, http.StatusBadRequest, "帖子 ID 不正确")
		return
	}

	var post Post
	if err := db.First(&post, uint(postID)).Error; err != nil {
		writeError(w, http.StatusNotFound, "找不到该帖子，可能已被删除")
		return
	}

	// 详情页允许未登录读取；如果已登录，就额外返回当前用户是否收藏。
	currentUser, hasLoginUser := currentUserFromRequest(r)
	enrichPostForResponse(&post, currentUser, hasLoginUser)
	writeJSON(w, http.StatusOK, post)
}

// ---------------------------------------------------------
// 7. 发布帖子接口
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
// 8. 删除帖子接口
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
// 9. 获取所有用户列表
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
// 10. 强制注销（删除）某个用户
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
// 11. 发表评论接口
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
// 12. 切换收藏状态接口 (点一下收藏，再点一下取消)
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
// 13. 获取我的收藏帖子
// ---------------------------------------------------------
func handleGetMyFavorites(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var favorites []Favorite
	if err := db.Where("uid = ?", user.UID).Find(&favorites).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "收藏列表读取失败")
		return
	}

	// 先取出收藏表里的 post_id，再一次性查询帖子，避免循环里反复查帖子影响性能。
	postIDs := make([]uint, 0, len(favorites))
	for _, fav := range favorites {
		postIDs = append(postIDs, fav.PostID)
	}
	if len(postIDs) == 0 {
		writeJSON(w, http.StatusOK, []Post{})
		return
	}

	var posts []Post
	if err := db.Where("id IN ?", postIDs).Order("created_at desc").Find(&posts).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "收藏帖子读取失败")
		return
	}

	for i := 0; i < len(posts); i++ {
		enrichPostForResponse(&posts[i], user, true)
	}

	writeJSON(w, http.StatusOK, posts)
}

// ---------------------------------------------------------
// 14. 删除评论接口
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

// ---------------------------------------------------------
// 15. 找回账号接口：邮箱 + 验证码换回用户名
// ---------------------------------------------------------
func handleRecoverAccount(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式不对")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	code := strings.TrimSpace(req.Code)
	if email == "" || code == "" {
		writeError(w, http.StatusBadRequest, "邮箱和验证码不能为空")
		return
	}

	if ok, message := verifyEmailCode(email, code); !ok {
		writeError(w, http.StatusUnauthorized, message)
		return
	}

	var user User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		writeError(w, http.StatusNotFound, "没有找到绑定该邮箱的账号")
		return
	}

	clearEmailCode(email)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "账号找回成功",
		"username": user.Username,
	})
}

// ---------------------------------------------------------
// 16. 重置密码接口：邮箱 + 验证码 + 新密码
// ---------------------------------------------------------
func handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Email       string `json:"email"`
		Code        string `json:"code"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式不对")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	code := strings.TrimSpace(req.Code)
	newPassword := strings.TrimSpace(req.NewPassword)
	if email == "" || code == "" || newPassword == "" {
		writeError(w, http.StatusBadRequest, "邮箱、验证码和新密码不能为空")
		return
	}
	if len(newPassword) < 6 {
		writeError(w, http.StatusBadRequest, "新密码至少需要 6 位")
		return
	}

	if ok, message := verifyEmailCode(email, code); !ok {
		writeError(w, http.StatusUnauthorized, message)
		return
	}

	var user User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		writeError(w, http.StatusNotFound, "没有找到绑定该邮箱的账号")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "新密码加密失败")
		return
	}

	user.PasswordHash = string(hashedPassword)
	if err := db.Save(&user).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "密码重置失败，数据库写入错误")
		return
	}

	clearEmailCode(email)
	writeJSON(w, http.StatusOK, map[string]string{"message": "密码已重置，请使用新密码登录"})
}

// ---------------------------------------------------------
// 17. 超级管理员资料更新接口
// ---------------------------------------------------------
func handleUpdateAdminProfile(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	admin, ok := requireAdmin(w, r)
	if !ok {
		return
	}

	var req struct {
		Username        string `json:"username"`
		Password        string `json:"password"`
		Avatar          string `json:"avatar"`
		Email           string `json:"email"`
		CurrentPassword string `json:"current_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "数据格式不对")
		return
	}

	newUsername := strings.TrimSpace(req.Username)
	newPassword := strings.TrimSpace(req.Password)
	newAvatar := strings.TrimSpace(req.Avatar)
	newEmail := strings.ToLower(strings.TrimSpace(req.Email))
	currentPassword := strings.TrimSpace(req.CurrentPassword)
	if currentPassword == "" {
		writeError(w, http.StatusForbidden, "修改管理员资料必须输入当前密码")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(currentPassword)); err != nil {
		writeError(w, http.StatusUnauthorized, "当前密码输入错误")
		return
	}

	oldUsername := admin.Username
	usernameChanged := false
	if newUsername != "" && newUsername != admin.Username {
		var existingUser User
		if err := db.Where("username = ? AND uid <> ?", newUsername, admin.UID).First(&existingUser).Error; err == nil {
			writeError(w, http.StatusBadRequest, "该管理员账号已被占用")
			return
		}
		admin.Username = newUsername
		admin.Nickname = newUsername
		usernameChanged = true
	}

	if newEmail != "" && newEmail != admin.Email {
		if !validateSupportedEmail(newEmail) {
			writeError(w, http.StatusForbidden, "目前仅支持 QQ 或 Gmail 邮箱")
			return
		}
		var existingUser User
		if err := db.Where("email = ? AND uid <> ?", newEmail, admin.UID).First(&existingUser).Error; err == nil {
			writeError(w, http.StatusBadRequest, "该邮箱已被其他账号绑定")
			return
		}
		admin.Email = newEmail
	}

	if newAvatar != "" {
		admin.Avatar = newAvatar
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
		admin.PasswordHash = string(hashedPassword)
	}

	if err := db.Save(&admin).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "管理员资料保存失败")
		return
	}

	if usernameChanged {
		db.Model(&Post{}).Where("username = ?", oldUsername).Update("username", admin.Username)
		db.Model(&Comment{}).Where("username = ?", oldUsername).Update("username", admin.Username)
	}

	token, err := generateToken(admin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "登录凭证刷新失败")
		return
	}

	payload := publicUserPayload(admin)
	payload["email"] = admin.Email
	payload["message"] = "管理员资料已更新"
	payload["token"] = token
	writeJSON(w, http.StatusOK, payload)
}
