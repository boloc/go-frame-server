// Package options 同时提供按插入顺序的 Option 列表和 O(1) 查找的 map。
package options

import (
	"cmp"
	"sort"
)

// Option 通用的下拉选项条目：Value 是真实值（枚举值/主键之类），Label 是展示文案。
type Option[K comparable, V any] struct {
	Value K `json:"value"`
	Label V `json:"label"`
}

// Builder 同时维护插入顺序（Options）和快速查找（Map/Contains）。展示顺序等于 Put 顺序。
type Builder[K comparable, V any] struct {
	order []K
	index map[K]V
}

// NewBuilder 创建一个 Builder。
func NewBuilder[K comparable, V any]() *Builder[K, V] {
	return &Builder[K, V]{index: make(map[K]V)}
}

// Put 添加一项，支持链式调用。同一 value 再次 Put 只更新 Label，不改变展示顺序。
func (b *Builder[K, V]) Put(value K, label V) *Builder[K, V] {
	if _, exists := b.index[value]; !exists {
		b.order = append(b.order, value)
	}
	b.index[value] = label
	return b
}

// Options 按 Put 顺序返回选项切片。
func (b *Builder[K, V]) Options() []Option[K, V] {
	out := make([]Option[K, V], len(b.order))
	for i, k := range b.order {
		out[i] = Option[K, V]{Value: k, Label: b.index[k]}
	}
	return out
}

// Get 返回某个 value 对应的 Label；未 Put 过时 ok 为 false。
func (b *Builder[K, V]) Get(value K) (V, bool) {
	v, ok := b.index[value]
	return v, ok
}

// Map 返回可安全修改的 map 副本，修改不影响 Builder 内部状态。
func (b *Builder[K, V]) Map() map[K]V {
	out := make(map[K]V, len(b.index))
	for k, v := range b.index {
		out[k] = v
	}
	return out
}

// Contains 判断某个 value 是否存在。
func (b *Builder[K, V]) Contains(value K) bool {
	_, ok := b.index[value]
	return ok
}

// SortByValue 按 Value 就地排序。K 须满足 cmp.Ordered。
// Options 默认已是 Put 顺序，仅在需要按值排序时调用。
func SortByValue[K cmp.Ordered, V any](opts []Option[K, V]) {
	sort.Slice(opts, func(i, j int) bool { return opts[i].Value < opts[j].Value })
}
