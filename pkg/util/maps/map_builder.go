package maps

// MapBuilder 构建常量映射表的工具
type MapBuilder[K comparable, V any] struct {
	mapping map[K]V
}

// NewMapBuilder 创建一个新的映射构建器
func NewMapBuilder[K comparable, V any]() *MapBuilder[K, V] {
	return &MapBuilder[K, V]{
		mapping: make(map[K]V),
	}
}

// Put 添加一个键值对
func (m *MapBuilder[K, V]) Put(key K, value V) *MapBuilder[K, V] {
	m.mapping[key] = value
	return m
}

// PutAll 批量添加键值对
func (m *MapBuilder[K, V]) PutAll(entries map[K]V) *MapBuilder[K, V] {
	for k, v := range entries {
		m.mapping[k] = v
	}
	return m
}

// Build 构建最终的映射
func (m *MapBuilder[K, V]) Build() map[K]V {
	return m.mapping
}

// BuildImmutable 构建不可变映射（返回一个获取函数）
func (m *MapBuilder[K, V]) BuildImmutable() func(key K) (V, bool) {
	finalMap := m.mapping
	return func(key K) (V, bool) {
		val, ok := finalMap[key]
		return val, ok
	}
}
