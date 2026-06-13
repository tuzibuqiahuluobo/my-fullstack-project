package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

const tokenTTL = 24 * time.Hour

const defaultTokenSecret = "dev-only-change-me"

type AuthClaims struct {
	UID       uint  `json:"uid"`
	ExpiresAt int64 `json:"exp"`
}

func corsAllowedOrigin() string {
	// 这里默认允许所有来源，是为了让初学阶段的前后端本地联调更省心。
	// 真正部署上线时，请设置 CORS_ALLOWED_ORIGIN 为你的前端域名，例如 https://example.com。
	return getEnv("CORS_ALLOWED_ORIGIN", "*")
}

func setJSONHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", corsAllowedOrigin())
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

func handleOptions(w http.ResponseWriter, r *http.Request) bool {
	setJSONHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func requireMethod(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	if handleOptions(w, r) {
		return false
	}
	for _, method := range allowed {
		if r.Method == method {
			return true
		}
	}
	writeError(w, http.StatusMethodNotAllowed, "请求方法不被允许")
	return false
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	setJSONHeaders(w)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func tokenSecret() []byte {
	// APP_TOKEN_SECRET 是 token 的签名密钥。开发时可以使用默认值，
	// 但上线必须改成足够长的随机字符串，否则别人可能伪造登录凭证。
	return []byte(getEnv("APP_TOKEN_SECRET", defaultTokenSecret))
}

func signTokenPayload(payload string) string {
	// HMAC 会把 payload 和密钥一起计算成签名；后端之后会重新计算一次，
	// 如果两次签名不同，就说明 token 被改过，应该拒绝请求。
	mac := hmac.New(sha256.New, tokenSecret())
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func generateToken(user User) (string, error) {
	// 这个项目使用的是学习友好的自定义 token：base64(payload).signature。
	// payload 里只放用户 UID 和过期时间，不放密码、邮箱等敏感信息。
	claims := AuthClaims{
		UID:       user.UID,
		ExpiresAt: time.Now().Add(tokenTTL).Unix(),
	}
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signature := signTokenPayload(payload)
	return payload + "." + signature, nil
}

func parseToken(token string) (AuthClaims, error) {
	var claims AuthClaims
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return claims, errors.New("token format is invalid")
	}

	// 先验签再解析 payload，避免信任被前端或攻击者篡改过的数据。
	expectedSignature := signTokenPayload(parts[0])
	if !hmac.Equal([]byte(expectedSignature), []byte(parts[1])) {
		return claims, errors.New("token signature is invalid")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, err
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return claims, err
	}
	if claims.UID == 0 || time.Now().Unix() > claims.ExpiresAt {
		return claims, errors.New("token is expired")
	}
	return claims, nil
}

func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func currentUserFromRequest(r *http.Request) (User, bool) {
	token := bearerToken(r)
	if token == "" {
		return User{}, false
	}

	claims, err := parseToken(token)
	if err != nil {
		return User{}, false
	}

	var user User
	if err := db.First(&user, claims.UID).Error; err != nil {
		return User{}, false
	}
	return user, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (User, bool) {
	user, ok := currentUserFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录，或重新登录后再试")
		return User{}, false
	}
	return user, true
}

func requireAdmin(w http.ResponseWriter, r *http.Request) (User, bool) {
	user, ok := requireUser(w, r)
	if !ok {
		return User{}, false
	}
	if user.Role != 2 {
		writeError(w, http.StatusForbidden, "需要超级管理员权限")
		return User{}, false
	}
	return user, true
}
