package ioutil

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadJSONFile 从文件加载 JSON 并解析到目标类型 T。
func LoadJSONFile[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件 %s 失败: %w", path, err)
	}

	var target T
	if err := json.Unmarshal(data, &target); err != nil {
		return nil, fmt.Errorf("解析文件 %s 失败: %w", path, err)
	}

	return &target, nil
}

// SaveJSONFile 将数据以带缩进的 JSON 格式写入到指定文件。
func SaveJSONFile(path string, data any) error {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 JSON 失败: %w", err)
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("写入文件 %s 失败: %w", path, err)
	}

	return nil
}
