package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) {
	t.Helper()

	var err error
	// 每个测试都使用内存数据库，这样不会读取或覆盖项目目录里的 data.db。
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &Topic{}, &Post{}, &Comment{}, &Favorite{}); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}
	seedDefaultTopicsAndMigratePosts()
}

func defaultTopicID(t *testing.T) uint {
	t.Helper()

	topic, ok := findDefaultTopic()
	if !ok {
		t.Fatalf("测试需要默认话题，但没有找到综合社区")
	}
	return topic.ID
}

func createTestUser(t *testing.T, username string, role int) User {
	t.Helper()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("生成测试密码哈希失败: %v", err)
	}

	user := User{
		Username:     username,
		Nickname:     username,
		Email:        username + "@qq.com",
		PasswordHash: string(passwordHash),
		Role:         role,
		Avatar:       "https://example.com/avatar.png",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	return user
}

func newJSONRequest(t *testing.T, method string, path string, body interface{}) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	setupTestDB(t)
	createTestUser(t, "alice", 0)

	req := newJSONRequest(t, http.MethodPost, "/api/login", map[string]string{
		"username": "alice",
		"password": "Wrong123",
	})
	rec := httptest.NewRecorder()

	handleLogin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("期望状态码 401，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
}

func TestValidatePasswordAllowsDotSpecialChar(t *testing.T) {
	if message := validatePassword("Secret123."); message != "" {
		t.Fatalf("期望点号可以作为密码特殊字符，实际错误: %s", message)
	}
}

func TestRegisterRejectsInvalidInputLimits(t *testing.T) {
	setupTestDB(t)

	tests := []struct {
		name string
		body map[string]string
	}{
		{
			name: "username too short",
			body: map[string]string{"username": "ab", "password": "Secret123", "email": "short@qq.com", "code": "123456"},
		},
		{
			name: "username does not start with letter",
			body: map[string]string{"username": "1user", "password": "Secret123", "email": "start@qq.com", "code": "123456"},
		},
		{
			name: "username contains sensitive word",
			body: map[string]string{"username": "adminUser", "password": "Secret123", "email": "sensitive@qq.com", "code": "123456"},
		},
		{
			name: "password too long",
			body: map[string]string{"username": "validuser", "password": "Aa123456789012345678901234567890123", "email": "long@qq.com", "code": "123456"},
		},
		{
			name: "password missing number",
			body: map[string]string{"username": "validuser", "password": "Password", "email": "letters@qq.com", "code": "123456"},
		},
		{
			name: "password missing letter",
			body: map[string]string{"username": "validuser", "password": "12345678", "email": "digits@qq.com", "code": "123456"},
		},
		{
			name: "password missing uppercase",
			body: map[string]string{"username": "validuser", "password": "secret123", "email": "upper@qq.com", "code": "123456"},
		},
		{
			name: "password has unsupported special char",
			body: map[string]string{"username": "validuser", "password": "Secret123?", "email": "special@qq.com", "code": "123456"},
		},
		{
			name: "unsupported email",
			body: map[string]string{"username": "validuser", "password": "Secret123", "email": "user@example.com", "code": "123456"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newJSONRequest(t, http.MethodPost, "/api/register", tt.body)
			rec := httptest.NewRecorder()

			handleRegister(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("期望状态码 400，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestUpdateRejectsLongNickname(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "nick_user", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodPost, "/api/update", map[string]string{
		"nickname": "这是一个明确超过十五个字的昵称内容",
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望状态码 400，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateRejectsSensitiveSignature(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "sign_user", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}

	signature := "这里有诈骗信息"
	req := newJSONRequest(t, http.MethodPost, "/api/update", UpdateRequest{
		Signature: &signature,
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望状态码 400，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
}

func TestRequireMethodRejectsWrongMethod(t *testing.T) {
	req := newJSONRequest(t, http.MethodGet, "/api/login", nil)
	rec := httptest.NewRecorder()

	handleLogin(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("期望状态码 405，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
}

func TestProtectedEndpointRejectsAnonymousUser(t *testing.T) {
	setupTestDB(t)

	req := newJSONRequest(t, http.MethodPost, "/api/create-post", map[string]string{
		"content": "hello",
	})
	rec := httptest.NewRecorder()

	handleCreatePost(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("期望状态码 401，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePostAllowsImageOnly(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "image_user", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodPost, "/api/create-post", map[string]interface{}{
		"topic_id": defaultTopicID(t),
		"image":    "data:image/png;base64,aGVsbG8=",
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleCreatePost(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
	var post Post
	if err := db.First(&post).Error; err != nil {
		t.Fatalf("期望图片帖子被保存，实际查询失败: %v", err)
	}
	if post.Image == "" {
		t.Fatalf("期望保存帖子图片，实际为空")
	}
}

func TestCreatePostAllowsMultipleImages(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "multi_image_user", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodPost, "/api/create-post", map[string]interface{}{
		"title":    "旅行记录",
		"content":  "今天看到了很好看的天空",
		"topic_id": defaultTopicID(t),
		"images": []string{
			"data:image/png;base64,aGVsbG8=",
			"data:image/webp;base64,d29ybGQ=",
		},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleCreatePost(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
	var post Post
	if err := db.First(&post).Error; err != nil {
		t.Fatalf("期望多图帖子被保存，实际查询失败: %v", err)
	}
	enrichPostForResponse(&post, user, true)
	if post.Title != "旅行记录" || len(post.Images) != 2 {
		t.Fatalf("期望保存标题和 2 张图片，实际标题=%q 图片数=%d", post.Title, len(post.Images))
	}
}

func TestCreatePostDefaultsToGeneralTopicAndStoresTags(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "tag_post_user", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodPost, "/api/create-post", map[string]interface{}{
		"content": "hello tags",
		"tags":    []string{" 绘画 ", "#灵感", "绘画"},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleCreatePost(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
	var post Post
	if err := db.First(&post).Error; err != nil {
		t.Fatalf("读取帖子失败: %v", err)
	}
	enrichPostForResponse(&post, user, true)
	if post.TopicID != defaultTopicID(t) {
		t.Fatalf("未传话题时应归入综合社区，实际 topic_id=%d", post.TopicID)
	}
	if len(post.Tags) != 2 || post.Tags[0] != "绘画" || post.Tags[1] != "灵感" {
		t.Fatalf("期望标签被清理并去重，实际: %#v", post.Tags)
	}
}

func TestCreatePostRejectsTooManyTags(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "too_many_tag_user", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodPost, "/api/create-post", map[string]interface{}{
		"topic_id": defaultTopicID(t),
		"content":  "hello",
		"tags":     []string{"a", "b", "c", "d", "e", "f"},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleCreatePost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望状态码 400，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePostRejectsTooManyImages(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "too_many_image_user", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}

	images := make([]string, 10)
	for i := range images {
		images[i] = "data:image/png;base64,aGVsbG8="
	}
	req := newJSONRequest(t, http.MethodPost, "/api/create-post", map[string]interface{}{
		"content":  "hello",
		"topic_id": defaultTopicID(t),
		"images":   images,
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleCreatePost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望状态码 400，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePostRejectsEmptyContentAndImage(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "empty_post_user", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodPost, "/api/create-post", map[string]interface{}{
		"topic_id": defaultTopicID(t),
		"content":  "   ",
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleCreatePost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望状态码 400，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePostRejectsInvalidImageData(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "bad_image_user", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodPost, "/api/create-post", map[string]interface{}{
		"topic_id": defaultTopicID(t),
		"content":  "hello",
		"image":    "data:image/png;base64,not-base64!!!",
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleCreatePost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望状态码 400，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdatePostAllowsAuthorToEditTitleContentAndImages(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "edit_post_user", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}
	post := Post{
		Username:  user.Username,
		TopicID:   defaultTopicID(t),
		Content:   "old content",
		CreatedAt: user.UsernameUpdatedAt,
	}
	if err := db.Create(&post).Error; err != nil {
		t.Fatalf("创建测试帖子失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodPost, "/api/update-post", map[string]interface{}{
		"post_id":  post.ID,
		"topic_id": defaultTopicID(t),
		"title":    "新标题",
		"content":  "new content",
		"images":   []string{"data:image/png;base64,aGVsbG8="},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleUpdatePost(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
	var updated Post
	if err := db.First(&updated, post.ID).Error; err != nil {
		t.Fatalf("读取编辑后的帖子失败: %v", err)
	}
	enrichPostForResponse(&updated, user, true)
	if updated.Title != "新标题" || updated.Content != "new content" || len(updated.Images) != 1 {
		t.Fatalf("期望帖子内容被更新，实际: 标题=%q 正文=%q 图片数=%d", updated.Title, updated.Content, len(updated.Images))
	}
}

func TestAdminEndpointRejectsNormalUser(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "bob", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleGetUsers(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("期望状态码 403，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
}

func TestPublicTopicsOnlyReturnApproved(t *testing.T) {
	setupTestDB(t)
	if err := db.Create(&Topic{Name: "Hidden Topic", Description: "disabled", SortOrder: 20, Status: TopicStatusDisabled}).Error; err != nil {
		t.Fatalf("创建停用话题失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodGet, "/api/topics", nil)
	rec := httptest.NewRecorder()

	handleGetTopics(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
	var topics []Topic
	if err := json.Unmarshal(rec.Body.Bytes(), &topics); err != nil {
		t.Fatalf("解析话题响应失败: %v", err)
	}
	for _, topic := range topics {
		if topic.Status != TopicStatusApproved {
			t.Fatalf("普通接口不应该返回未通过话题，实际得到: %+v", topic)
		}
	}
}

func TestAdminTopicEndpointRejectsNormalUser(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "topic_normal_user", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodGet, "/api/admin/topics", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleAdminGetTopics(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("期望状态码 403，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePostRejectsDisabledTopic(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "disabled_topic_user", 0)
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}
	disabled := Topic{Name: "Disabled Topic", Description: "not visible", SortOrder: 30, Status: TopicStatusDisabled}
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatalf("创建停用话题失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodPost, "/api/create-post", map[string]interface{}{
		"topic_id": disabled.ID,
		"content":  "hello",
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleCreatePost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望状态码 400，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminDeleteTopicMigratesPostsToDefaultTopic(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	admin := createTestUser(t, "topic_admin_user", 2)
	token, err := generateToken(admin)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}
	topic := Topic{Name: "Temporary Topic", Description: "will be deleted", SortOrder: 40, Status: TopicStatusApproved}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatalf("创建测试话题失败: %v", err)
	}
	post := Post{Username: admin.Username, TopicID: topic.ID, Content: "need migrate"}
	if err := db.Create(&post).Error; err != nil {
		t.Fatalf("创建测试帖子失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodPost, "/api/admin/topics/delete", map[string]interface{}{"topic_id": topic.ID})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleAdminDeleteTopic(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
	var migrated Post
	if err := db.First(&migrated, post.ID).Error; err != nil {
		t.Fatalf("读取迁移后的帖子失败: %v", err)
	}
	if migrated.TopicID != defaultTopicID(t) {
		t.Fatalf("期望帖子迁移到综合社区，实际 topic_id=%d", migrated.TopicID)
	}
}

func TestGetCurrentUserReturnsSignature(t *testing.T) {
	setupTestDB(t)
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-api")

	user := createTestUser(t, "sign_user", 0)
	user.Signature = "今天也要闪闪发光"
	if err := db.Save(&user).Error; err != nil {
		t.Fatalf("保存测试个性签名失败: %v", err)
	}
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("生成测试 token 失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handleGetCurrentUser(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if body["signature"] != "今天也要闪闪发光" {
		t.Fatalf("期望返回最新个性签名，实际得到 %v", body["signature"])
	}
}

func TestGetPostsIncludesAuthorSignature(t *testing.T) {
	setupTestDB(t)

	user := createTestUser(t, "post_author", 0)
	user.Signature = "在社区留下小小脚印"
	if err := db.Save(&user).Error; err != nil {
		t.Fatalf("保存测试个性签名失败: %v", err)
	}
	if err := db.Create(&Post{
		Username:  user.Username,
		TopicID:   defaultTopicID(t),
		Content:   "hello sunshine",
		CreatedAt: user.UsernameUpdatedAt,
	}).Error; err != nil {
		t.Fatalf("创建测试帖子失败: %v", err)
	}

	req := newJSONRequest(t, http.MethodGet, "/api/posts", nil)
	rec := httptest.NewRecorder()

	handleGetPosts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
	var posts []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &posts); err != nil {
		t.Fatalf("解析帖子响应失败: %v", err)
	}
	if len(posts) != 1 || posts[0]["signature"] != "在社区留下小小脚印" {
		t.Fatalf("期望帖子带出作者个性签名，实际响应: %v", posts)
	}
}

func TestRecoverAccountReturnsUsernameWithValidCode(t *testing.T) {
	setupTestDB(t)
	createTestUser(t, "sunny", 0)
	saveEmailCode("sunny@qq.com", "123456")

	req := newJSONRequest(t, http.MethodPost, "/api/recover-account", map[string]string{
		"email": "sunny@qq.com",
		"code":  "123456",
	})
	rec := httptest.NewRecorder()

	handleRecoverAccount(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if body["username"] != "sunny" {
		t.Fatalf("期望找回账号 sunny，实际得到 %v", body["username"])
	}
}

func TestResetPasswordAllowsLoginWithNewPassword(t *testing.T) {
	setupTestDB(t)
	createTestUser(t, "rainy", 0)
	saveEmailCode("rainy@qq.com", "654321")

	req := newJSONRequest(t, http.MethodPost, "/api/reset-password", map[string]string{
		"email":        "rainy@qq.com",
		"code":         "654321",
		"new_password": "Newpass123",
	})
	rec := httptest.NewRecorder()

	handleResetPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}

	loginReq := newJSONRequest(t, http.MethodPost, "/api/login", map[string]string{
		"username": "rainy",
		"password": "Newpass123",
	})
	loginRec := httptest.NewRecorder()
	handleLogin(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("重置密码后登录失败，状态码 %d，响应: %s", loginRec.Code, loginRec.Body.String())
	}
}
