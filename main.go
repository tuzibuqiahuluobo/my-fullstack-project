package main

import (
	"fmt"
	"net/http"
)

func main() {
	// 启动时读取本地 .env 文件，这样你只需要在 .env 里填写邮箱授权码，不必每次手动敲环境变量。
	loadDotEnv(".env")

	// 1. 初始化数据库 (会自动调用 models.go 里的函数)
	initDB()

	// 2. 把前端发来的请求，精准派发给 api.go 里的接口函数
	mux := http.NewServeMux()
	// 使用独立 mux 是为了把所有接口统一包进 CORS 中间件，前端跨端口访问时不会漏响应头。
	mux.HandleFunc("/api/register", handleRegister)
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/update", handleUpdate)
	mux.HandleFunc("/api/send-code", handleSendCode)
	mux.HandleFunc("/api/recover-account", handleRecoverAccount)
	mux.HandleFunc("/api/reset-password", handleResetPassword)
	mux.HandleFunc("/api/update-admin-profile", handleUpdateAdminProfile)
	mux.HandleFunc("/api/posts", handleGetPosts)
	mux.HandleFunc("/api/post-detail", handleGetPostDetail)
	mux.HandleFunc("/api/create-post", handleCreatePost)
	mux.HandleFunc("/api/delete-post", handleDeletePost)
	mux.HandleFunc("/api/users", handleGetUsers)
	mux.HandleFunc("/api/delete-user", handleDeleteUser)
	mux.HandleFunc("/api/create-comment", handleCreateComment)
	mux.HandleFunc("/api/toggle-favorite", handleToggleFavorite)
	mux.HandleFunc("/api/my-favorites", handleGetMyFavorites)
	mux.HandleFunc("/api/delete-comment", handleDeleteComment)

	// 3. 启动服务器
	fmt.Println("🚀 服务器已启动！运行在 http://localhost:8080")
	if err := http.ListenAndServe(":8080", withCORS(mux)); err != nil {
		fmt.Println("服务器启动失败:", err)
	}
}
