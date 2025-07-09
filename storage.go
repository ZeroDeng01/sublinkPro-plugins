package plugins

// PluginStorage 插件存储接口
type PluginStorage interface {
	// GetPlugin 根据路径获取插件信息
	GetPlugin(path string) (*PluginStorageInfo, error)

	// SavePlugin 保存或更新插件信息
	SavePlugin(name, path string, enabled bool, config map[string]interface{}) error
}

// PluginStorageInfo 插件存储信息
type PluginStorageInfo struct {
	Name    string
	Path    string
	Enabled bool
	Config  string // JSON格式的配置
}

// DefaultStorage 默认的存储实现（空实现）
type DefaultStorage struct{}

func (d *DefaultStorage) GetPlugin(path string) (*PluginStorageInfo, error) {
	return nil, nil // 返回nil表示插件不存在
}

func (d *DefaultStorage) SavePlugin(name, path string, enabled bool, config map[string]interface{}) error {
	return nil // 空实现，不执行任何操作
}

// 全局存储实例
var storage PluginStorage = &DefaultStorage{}

// SetStorage 设置存储实现
func SetStorage(s PluginStorage) {
	storage = s
}

// GetStorage 获取存储实现
func GetStorage() PluginStorage {
	return storage
}
