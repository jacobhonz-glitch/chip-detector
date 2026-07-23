// chip-detector/main.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"chip-detector/pkg/api"
)

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}

func main() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	router := api.NewRouter()

	httpServer := &http.Server{
		Addr:    ":19527",
		Handler: router,
	}

	go func() {
		fmt.Println("========================================")
		fmt.Println("  Chip Detector v0.2 - 芯片自动识别")
		fmt.Println("========================================")
		fmt.Println("  http://localhost:19527")
		fmt.Println("========================================")

		// 自动打开浏览器
		openBrowser("http://localhost:19527")

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	<-sigChan
	fmt.Println("\n正在关闭...")
}