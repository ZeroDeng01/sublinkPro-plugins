package plugins

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"plugin"
	"runtime"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// Manager 插件管理器
type Manager struct {
	plugins   map[string]*PluginInfo
	pluginDir string
	mutex     sync.RWMutex
}

var (
	manager *Manager
	once    sync.Once
)

// GetManager 获取插件管理器实例（单例）
func GetManager() *Manager {
	once.Do(func() {
		manager = &Manager{
			plugins:   make(map[string]*PluginInfo),
			pluginDir: "./plugins",
		}
	})
	return manager
}

// LoadPlugins 加载所有插件
func (m *Manager) LoadPlugins() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 确保插件目录存在
	if _, err := os.Stat(m.pluginDir); os.IsNotExist(err) {
		if err := os.MkdirAll(m.pluginDir, 0755); err != nil {
			return fmt.Errorf("创建插件目录失败: %v", err)
		}
		log.Printf("创建插件目录: %s", m.pluginDir)
		return nil
	}

	// 遍历插件目录
	return filepath.Walk(m.pluginDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只加载.so文件（编译后的插件）
		if strings.HasSuffix(path, ".so") {
			if err := m.loadPlugin(path); err != nil {
				log.Printf("加载插件失败 %s: %v", path, err)
				// 继续加载其他插件
			}
		}

		return nil
	})
}

// loadPlugin 加载单个插件
func (m *Manager) loadPlugin(pluginPath string) error {


	p, err := plugin.Open(pluginPath)
	if err != nil {
		fmt.Printf("插件加载失败，详细错误: %v\n", err)
		return fmt.Errorf("打开插件失败: %w", err)
	}

	// 查找GetPlugin函数
	symGetPlugin, err := p.Lookup("GetPlugin")
	if err != nil {
		return fmt.Errorf("找不到GetPlugin函数: %v", err)
	}

	// 类型断言为函数
	getPlugin, ok := symGetPlugin.(func() Plugin)
	if !ok {
		return fmt.Errorf("GetPlugin函数签名不正确")
	}

	// 获取插件实例
	pluginInstance := getPlugin()

	// 从存储中获取插件信息
	pluginDB, _ := storage.GetPlugin(pluginPath)

	if pluginDB == nil {
		log.Printf("插件 %s 未在数据库中注册", pluginPath)
	}

	// 初始化配置
	var config map[string]interface{}
	var enable bool

	if pluginDB != nil {
		if pluginDB.Config != "" {
			err := json.Unmarshal([]byte(pluginDB.Config), &config)
			if err != nil {
				return fmt.Errorf("解析插件配置失败: %v", err)
			}
		}

		enable = pluginDB.Enabled
	} else {
		// 如果数据库中没有配置,则使用默认配置
		config = pluginInstance.DefaultConfig()
		enable = false
	}

	// 设置配置到插件
	pluginInstance.SetConfig(config)

	// 创建插件信息
	info := &PluginInfo{
		Name:        pluginInstance.Name(),
		Version:     pluginInstance.Version(),
		Description: pluginInstance.Description(),
		FilePath:    pluginPath,
		Enabled:     enable,
		Config:      config,
		Plugin:      pluginInstance,
	}

	// 如果插件已启用，则初始化插件
	if info.Enabled {
		if err := pluginInstance.Init(); err != nil {
			log.Printf("初始化插件 %s 失败: %v", info.Name, err)
			info.Enabled = false

			// 同步插件状态到存储
			if err := storage.SavePlugin(info.Name, pluginPath, false, info.Config); err != nil {
				log.Printf("更新插件状态到存储失败: %v", err)
			}

			// 初始化失败，跳过当前插件加载
			return fmt.Errorf("初始化插件 %s 失败，跳过加载", info.Name)
		}
	}

	// 存储插件
	m.plugins[info.Name] = info

	// 同步插件信息到存储
	if err := storage.SavePlugin(info.Name, pluginPath, info.Enabled, info.Config); err != nil {
		log.Printf("保存插件信息到存储失败: %v", err)
	}

	log.Printf("成功加载插件: %s v%s", info.Name, info.Version)
	return nil
}

// GetPlugin 获取插件
func (m *Manager) GetPlugin(name string) (*PluginInfo, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	plugin, exists := m.plugins[name]
	return plugin, exists
}

// GetAllPlugins 获取所有插件
func (m *Manager) GetAllPlugins() map[string]*PluginInfo {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[string]*PluginInfo)
	for k, v := range m.plugins {
		result[k] = v
	}
	return result
}

// EnablePlugin 启用插件
func (m *Manager) EnablePlugin(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("插件不存在: %s", name)
	}

	// 如果插件已经启用，则不需要重复操作
	if plugin.Enabled {
		return nil
	}

	// 初始化插件
	if err := plugin.Plugin.Init(); err != nil {
		return fmt.Errorf("初始化插件失败: %v", err)
	}

	plugin.Enabled = true

	// 同步写入存储
	if err := storage.SavePlugin(plugin.Name, plugin.FilePath, true, plugin.Config); err != nil {
		// 如果存储更新失败，回滚内存状态并关闭已初始化的插件
		plugin.Enabled = false
		_ = plugin.Plugin.Close() // 忽略关闭错误，因为已经有更严重的存储错误
		return fmt.Errorf("更新插件状态到存储失败: %v", err)
	}

	return nil
}

// DisablePlugin 禁用插件
func (m *Manager) DisablePlugin(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("插件不存在: %s", name)
	}

	// 如果插件已经禁用，则不需要重复操作
	if !plugin.Enabled {
		return nil
	}

	// 关闭插件
	if err := plugin.Plugin.Close(); err != nil {
		// 即使关闭失败，我们也要将插件标记为禁用
		log.Printf("关闭插件 %s 失败: %v", name, err)
	}

	plugin.Enabled = false

	// 同步写入存储
	if err := storage.SavePlugin(plugin.Name, plugin.FilePath, false, plugin.Config); err != nil {
		// 如果存储更新失败，记录错误但不回滚状态（插件已经被关闭）
		log.Printf("更新插件状态到存储失败: %v", err)
		// 仍然返回成功，因为插件已成功禁用，只是存储同步失败
	}

	return nil
}

// UpdatePluginConfig 更新插件配置
func (m *Manager) UpdatePluginConfig(name string, config map[string]interface{}) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("插件不存在: %s", name)
	}

	// 先备份旧配置，以便回滚
	oldConfig := plugin.Config

	// 更新内存中的配置
	plugin.Config = config

	// 更新插件内部配置
	plugin.Plugin.SetConfig(config)

	// 同步写入存储
	if err := storage.SavePlugin(plugin.Name, plugin.FilePath, plugin.Enabled, config); err != nil {
		// 如果存储更新失败，回滚内存配置
		plugin.Config = oldConfig
		plugin.Plugin.SetConfig(oldConfig) // 尝试回滚插件内部配置
		return fmt.Errorf("更新插件配置到存储失败: %v", err)
	}

	return nil
}

// TriggerEvent 触发事件
func (m *Manager) TriggerEvent(ctx *gin.Context, event EventType, path string, statusCode int, requestBody interface{}, responseBody interface{}) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, pluginInfo := range m.plugins {
		if !pluginInfo.Enabled {
			continue
		}

		// 检查插件是否对这个事件感兴趣
		interestedEvents := pluginInfo.Plugin.InterestedEvents()
		eventInterested := false
		for _, interestedEvent := range interestedEvents {
			if interestedEvent == event {
				eventInterested = true
				break
			}
		}

		if !eventInterested {
			continue
		}

		// 检查插件是否对这个API路径感兴趣
		interestedAPIs := pluginInfo.Plugin.InterestedAPIs()
		apiInterested := false
		for _, interestedAPI := range interestedAPIs {
			if strings.HasPrefix(path, interestedAPI) {
				apiInterested = true
				break
			}
		}

		if !apiInterested {
			continue
		}

		// 执行插件事件处理
		go func(p Plugin, name string) {
			if err := p.OnAPIEvent(ctx, event, path, statusCode, requestBody, responseBody); err != nil {
				log.Printf("插件 %s 处理事件失败: %v", name, err)
			}
		}(pluginInfo.Plugin, pluginInfo.Name)
	}
}

// Shutdown 关闭所有插件
func (m *Manager) Shutdown() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, pluginInfo := range m.plugins {
		if err := pluginInfo.Plugin.Close(); err != nil {
			log.Printf("关闭插件失败 %s: %v", pluginInfo.Name, err)
		}
	}

	m.plugins = make(map[string]*PluginInfo)
}
