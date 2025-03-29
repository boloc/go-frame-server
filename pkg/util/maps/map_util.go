package maps

// SliceToMap 将任意切片转换为map
// T: 切片中元素类型, K: 键类型, V: 值类型
func SliceToMap[T any, K comparable, V any](slice []T, keyFn func(T) K, valFn func(T) V) map[K]V {

	result := make(map[K]V, len(slice))
	for _, item := range slice {
		result[keyFn(item)] = valFn(item)
	}
	return result
}

// StructSliceToMap 将结构体切片转换为map，键为结构体的某个字段，值为结构体本身
func StructSliceToMap[T any, K comparable](slice []T, keyFn func(T) K) map[K]T {
	result := make(map[K]T, len(slice))
	for _, item := range slice {
		result[keyFn(item)] = item
	}
	return result
}

// ExtractField 从结构体切片中提取某个字段组成新切片
func ExtractField[T any, V any](slice []T, extractFn func(T) V) []V {
	result := make([]V, len(slice))
	for i, item := range slice {
		result[i] = extractFn(item)
	}
	return result
}

// GetOrDefault 获取map中的值，如果不存在则返回默认值
func GetOrDefault[K comparable, V any](m map[K]V, key K, defaultVal V) V {
	if val, ok := m[key]; ok {
		return val
	}
	return defaultVal
}
