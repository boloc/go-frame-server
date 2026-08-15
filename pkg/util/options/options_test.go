package options

import (
	"testing"
)

// TestOptionsPreservesInsertionOrder 验证 Options() 按 Put 调用顺序返回，不按 Value 排序。
func TestOptionsPreservesInsertionOrder(t *testing.T) {
	b := NewBuilder[int, string]().
		Put(3, "视频").
		Put(1, "APP").
		Put(2, "交友")

	got := b.Options()
	want := []Option[int, string]{
		{Value: 3, Label: "视频"},
		{Value: 1, Label: "APP"},
		{Value: 2, Label: "交友"},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Options()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestPutDuplicateValueUpdatesLabelWithoutMovingPosition 验证重复 Put 只更新 Label，不改位置、不产生重复项。
func TestPutDuplicateValueUpdatesLabelWithoutMovingPosition(t *testing.T) {
	b := NewBuilder[int, string]().
		Put(1, "旧文案").
		Put(2, "第二项").
		Put(1, "新文案") // 重复 Put 同一个 value

	got := b.Options()
	if len(got) != 2 {
		t.Fatalf("重复 Put 同一个 value 不应该产生重复项，len = %d, want 2", len(got))
	}
	if got[0] != (Option[int, string]{Value: 1, Label: "新文案"}) {
		t.Fatalf("第一项应该保留原位置、更新成新文案，实际 %+v", got[0])
	}
	if got[1] != (Option[int, string]{Value: 2, Label: "第二项"}) {
		t.Fatalf("第二项不应该被影响，实际 %+v", got[1])
	}
}

// TestMapReturnsIndependentCopy 验证 Map() 返回副本，外部修改不影响 Builder。
func TestMapReturnsIndependentCopy(t *testing.T) {
	b := NewBuilder[int, string]().Put(1, "APP")

	m := b.Map()
	m[1] = "被外部改坏了"
	m[2] = "外部新增的"

	again := b.Map()
	if again[1] != "APP" {
		t.Fatalf("外部修改 Map() 返回值不应该影响 Builder 内部状态，again[1] = %q", again[1])
	}
	if _, ok := again[2]; ok {
		t.Fatal("外部新增的 key 不应该出现在 Builder 内部状态里")
	}
}

// TestGet 验证按 value 查找 Label。
func TestGet(t *testing.T) {
	b := NewBuilder[int, string]().Put(1, "APP")

	if label, ok := b.Get(1); !ok || label != "APP" {
		t.Fatalf("Get(1) = (%q, %v), want (\"APP\", true)", label, ok)
	}
	if _, ok := b.Get(999); ok {
		t.Fatal("Get(999) 应该是 ok=false")
	}
}

// TestContains 验证 Contains 的存在性判断。
func TestContains(t *testing.T) {
	b := NewBuilder[int, string]().Put(1, "APP").Put(2, "交友")

	if !b.Contains(1) {
		t.Fatal("Contains(1) 应该是 true")
	}
	if b.Contains(999) {
		t.Fatal("Contains(999) 应该是 false")
	}
}

// eventCode 是底层为 int 的命名类型，用于验证按数值而不是字符串排序。
type eventCode int

const (
	codeDebug        eventCode = 5000
	codeInstallPoint eventCode = 10001
	codeReturn       eventCode = 20003
)

// TestSortByValueHandlesNamedTypesNumerically 验证命名整数类型按数值排序，而不是按字符串。
func TestSortByValueHandlesNamedTypesNumerically(t *testing.T) {
	opts := []Option[eventCode, string]{
		{Value: codeReturn, Label: "返回量"},
		{Value: codeDebug, Label: "调试"},
		{Value: codeInstallPoint, Label: "安装点"},
	}

	SortByValue(opts)

	want := []eventCode{codeDebug, codeInstallPoint, codeReturn} // 5000 < 10001 < 20003
	for i, w := range want {
		if opts[i].Value != w {
			t.Fatalf("排序结果第 %d 项 = %d, want %d（数值顺序，不是字符串顺序）", i, opts[i].Value, w)
		}
	}
}

// TestSortByValueWithStringKeys 验证字符串 key 也能按字典序排序。
func TestSortByValueWithStringKeys(t *testing.T) {
	opts := []Option[string, string]{
		{Value: "b", Label: "第二"},
		{Value: "a", Label: "第一"},
	}
	SortByValue(opts)
	if opts[0].Value != "a" || opts[1].Value != "b" {
		t.Fatalf("排序结果不对: %+v", opts)
	}
}

// BenchmarkContains 测量 Contains 的查找开销。
func BenchmarkContains(b *testing.B) {
	builder := NewBuilder[int, string]()
	for i := 0; i < 100; i++ {
		builder.Put(i, "label")
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		builder.Contains(50)
	}
}
