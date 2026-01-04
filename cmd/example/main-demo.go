package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/boloc/go-frame-server/cmd/example/bootstrap"
	"github.com/boloc/go-frame-server/pkg/frame"
)

// 创建框架实例,初始化所有组件,注册启动后的操作,注册停止前的操作,运行框架
func main() {
	// 默认配置文件路径
	defaultConfigPath := "./config/frame-server.yml"
	f := frame.New(
		frame.WithConfigFile(defaultConfigPath),   // 优先级：环境变量 CONFIG_FILE > -c 命令行参数 > 默认值
		frame.WithShutdownTimeout(30*time.Second), // 设置30秒关闭超时
	)

	// 初始化所有组件
	bootstrap.Setup(f)

	// 注册启动后的操作
	f.AfterStart(func(ctx context.Context) error {
		fmt.Println("框架启动后执行...")
		return nil
	})

	// 注册停止前的操作
	f.BeforeStop(func(ctx context.Context) error {
		fmt.Println("框架停止前执行...")
		return nil
	})

	// 运行框架
	if err := f.Run(); err != nil {
		fmt.Println("Framework error", err)
		os.Exit(1)
	}
}
