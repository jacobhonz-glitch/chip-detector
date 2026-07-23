// chip-detector/main.go
// 芯片自动识别系统入口

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"chip-detector/pkg/api"
)

func main() {
	// 处理退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 创建路由
	router := api.NewRouter()

	httpServer := &http.Server{
		Addr:    ":19527",
		Handler: router,
	}

	// 启动服务
	go func() {
		fmt.Println("========================================")
		fmt.Println("  Chip Detector - 芯片自动识别系统 v0.2")
		fmt.Println("========================================")
		fmt.Printf("  Web界面 : http://localhost:19527\n")
		fmt.Printf("  API接口 : http://localhost:19527/api\n")
		fmt.Printf("  WebSocket : ws://localhost:19527/ws\n")
		fmt.Println("========================================")
		fmt.Println("  按 Ctrl+C 退出")
		fmt.Println()

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	// 等待退出信号
	<-sigChan
	fmt.Println("\n正在关闭服务...")
}