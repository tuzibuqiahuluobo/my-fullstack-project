package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGenerateAndParseToken(t *testing.T) {
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-token")

	token, err := generateToken(User{UID: 7})
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}

	claims, err := parseToken(token)
	if err != nil {
		t.Fatalf("解析 token 失败: %v", err)
	}
	if claims.UID != 7 {
		t.Fatalf("期望 UID 为 7，实际得到 %d", claims.UID)
	}
}

func TestParseTokenRejectsTamperedSignature(t *testing.T) {
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-token")

	token, err := generateToken(User{UID: 8})
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatalf("token 格式不符合预期: %s", token)
	}

	if _, err := parseToken(parts[0] + ".bad-signature"); err == nil {
		t.Fatal("签名被篡改后仍然解析成功，期望返回错误")
	}
}

func TestParseTokenRejectsExpiredToken(t *testing.T) {
	t.Setenv("APP_TOKEN_SECRET", "test-secret-for-token")

	claims := AuthClaims{
		UID:       9,
		ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	}
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("序列化测试 claims 失败: %v", err)
	}

	// 测试里手动拼一个过期 token，能直接验证 parseToken 的过期判断。
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	token := payload + "." + signTokenPayload(payload)

	if _, err := parseToken(token); err == nil {
		t.Fatal("过期 token 仍然解析成功，期望返回错误")
	}
}

func TestGetEnvUsesFallbackWhenEmpty(t *testing.T) {
	t.Setenv("APP_TEST_EMPTY_VALUE", "")

	if value := getEnv("APP_TEST_EMPTY_VALUE", "fallback-value"); value != "fallback-value" {
		t.Fatalf("期望读取 fallback-value，实际得到 %q", value)
	}
}

func TestCorsAllowedOriginCanBeConfigured(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGIN", "http://localhost:5173")

	if origin := corsAllowedOrigin(); origin != "http://localhost:5173" {
		t.Fatalf("期望 CORS 来源可由环境变量配置，实际得到 %q", origin)
	}
}
