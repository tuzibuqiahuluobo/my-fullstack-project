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
	if err := db.AutoMigrate(&User{}, &Post{}, &Comment{}, &Favorite{}); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}
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
		"password": "wrong-password",
	})
	rec := httptest.NewRecorder()

	handleLogin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("期望状态码 401，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
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
			body: map[string]string{"username": "a", "password": "secret123", "email": "short@qq.com", "code": "123456"},
		},
		{
			name: "password too long",
			body: map[string]string{"username": "valid_user", "password": "123456789012345678901234567890123", "email": "long@qq.com", "code": "123456"},
		},
		{
			name: "unsupported email",
			body: map[string]string{"username": "valid_user", "password": "secret123", "email": "user@example.com", "code": "123456"},
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
		"new_password": "newpass123",
	})
	rec := httptest.NewRecorder()

	handleResetPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际得到 %d，响应: %s", rec.Code, rec.Body.String())
	}

	loginReq := newJSONRequest(t, http.MethodPost, "/api/login", map[string]string{
		"username": "rainy",
		"password": "newpass123",
	})
	loginRec := httptest.NewRecorder()
	handleLogin(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("重置密码后登录失败，状态码 %d，响应: %s", loginRec.Code, loginRec.Body.String())
	}
}
