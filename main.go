package main

import (
	"fmt"
	"net/http"
)

func main() {
	// 1. 初始化数据库 (会自动调用 models.go 里的函数)
	initDB()

	// 2. 把前端发来的请求，精准派发给 api.go 里的接口函数
	http.HandleFunc("/api/register", handleRegister)
	http.HandleFunc("/api/login", handleLogin)
	http.HandleFunc("/api/update", handleUpdate)
	http.HandleFunc("/api/send-code", handleSendCode)
	http.HandleFunc("/api/posts", handleGetPosts)
	http.HandleFunc("/api/create-post", handleCreatePost)
	http.HandleFunc("/api/delete-post", handleDeletePost)
	http.HandleFunc("/api/users", handleGetUsers)
	http.HandleFunc("/api/delete-user", handleDeleteUser)
	http.HandleFunc("/api/create-comment", handleCreateComment)
	http.HandleFunc("/api/toggle-favorite", handleToggleFavorite)
	http.HandleFunc("/api/delete-comment", handleDeleteComment)

	// 3. 启动服务器
	fmt.Println("🚀 服务器已启动！运行在 http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("服务器启动失败:", err)
	}
}
