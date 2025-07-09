package plugins

import (
	"github.com/gin-gonic/gin"
)

// EventType 事件类型
type EventType string

const (
	EventAPISuccess EventType = "api_success"
	EventAPIError   EventType = "api_error"
	EventAPIBefore  EventType = "api_before"
	EventAPIAfter   EventType = "api_after"
)

// Plugin 插件接口
type Plugin interface {
	// Name 获取插件名称
	Name() string

	// Version 获取插件版本
	Version() string

	// Description 获取插件描述
	Description() string

	// DefaultConfig 返回默认配置
	DefaultConfig() map[string]interface{}

	// SetConfig 设置插件配置
	SetConfig(config map[string]interface{})

	// Init 初始化插件
	Init() error

	// Close 关闭插件
	Close() error

	// OnAPIEvent 处理API事件
	OnAPIEvent(ctx *gin.Context, event EventType, path string, statusCode int, requestBody interface{}, responseBody interface{}) error

	// InterestedAPIs 获取感兴趣的API路径
	InterestedAPIs() []string

	// InterestedEvents 获取感兴趣的事件类型
	InterestedEvents() []EventType
}

// PluginInfo 插件信息
type PluginInfo struct {
	Name        string
	Version     string
	Description string
	FilePath    string
	Enabled     bool
	Config      map[string]interface{}
	Plugin      Plugin
}
