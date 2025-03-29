package tests

import (
	"fmt"
	"os"
	"testing"

	"github.com/boloc/go-frame-server/pkg/util/maps"
)

type Option[K comparable, V any] struct {
	Key   K
	Value V
}

// 测试主函数(固定写法)
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// 测试SliceToMap函数
// go test -v -run TestSliceToMap  ./tests/maps_test.go
func TestSliceToMap(t *testing.T) {
	key := "nav1"
	// 定义一个切片*Option[string, string]
	navOptions := []*Option[string, string]{
		{Key: "nav1", Value: "首页导航"},
		{Key: "nav2", Value: "底部导航"},
		{Key: "nav3", Value: "侧边栏"},
	}

	// 使用工具函数转换为map
	navMap := maps.SliceToMap(navOptions,
		func(opt *Option[string, string]) string { return opt.Key },
		func(opt *Option[string, string]) string { return opt.Value },
	)
	// 获取导航映射
	nav1Tab := navMap[key]
	fmt.Println("导航映射:", nav1Tab)
}

// 测试ExtractField函数
// 获取所有key/value
// go test -v -run TestExtractField  ./tests/maps_test.go
func TestExtractField(t *testing.T) {
	navOptions := []*Option[string, string]{
		{Key: "nav1", Value: "首页导航"},
		{Key: "nav2", Value: "底部导航"},
		{Key: "nav3", Value: "侧边栏"},
	}

	keys := maps.ExtractField(navOptions, func(opt *Option[string, string]) string { return opt.Key })
	fmt.Println("ALL Keys:", keys)

	values := maps.ExtractField(navOptions, func(opt *Option[string, string]) string { return opt.Value })
	fmt.Println("ALL Values:", values)
}

// 测试NewMapBuilder 函数 测试1
// go test -v -run TestCustomMapBuilder1  ./tests/maps_test.go
func TestCustomMapBuilder1(t *testing.T) {
	navOptions := []*Option[string, string]{
		{Key: "nav1", Value: "首页导航"},
		{Key: "nav2", Value: "底部导航"},
		{Key: "nav3", Value: "侧边栏-1"},
		{Key: "nav4", Value: "侧边栏-2"},
		{Key: "nav5", Value: "侧边栏-3"},
	}

	builder := maps.NewMapBuilder[string, string]()
	for _, opt := range navOptions {
		builder.Put(opt.Key, opt.Value)
	}
	navMap := builder.Build()
	fmt.Println("导航映射1:", navMap)

	// 批量添加
	// 将navOptions转换为map[string]string
	navMap2 := maps.SliceToMap(navOptions,
		func(opt *Option[string, string]) string { return opt.Key },
		func(opt *Option[string, string]) string { return opt.Value },
	)
	builder.PutAll(navMap2)
	navMap3 := builder.Build()
	fmt.Println("导航映射2的首页导航:", navMap3["nav1"])
}

// 测试NewMapBuilder 函数 测试2
// go test -v -run TestCustomMapBuilder2  ./tests/maps_test.go
func TestCustomMapBuilder2(t *testing.T) {
	userRoleMap := maps.NewMapBuilder[int, string]().
		Put(1, "管理员").
		Put(2, "编辑").
		Put(3, "访客1").
		PutAll(map[int]string{
			4: "游客2",
			5: "游客3",
		}).
		Build()
	fmt.Println("用户角色映射:", userRoleMap)
}

// 测试业务map
// go test -v -run TestBusinessMap  ./tests/maps_test.go
func TestBusinessMap(t *testing.T) {
	type Data struct {
		ID     int
		Name   string
		NavID  int
		Status int
	}

	DataList := []Data{
		{ID: 3, Name: "首页推荐", NavID: 1, Status: 1},
		{ID: 4, Name: "热门推荐", NavID: 2, Status: 0},
	}
	// 测试1，存入整个结构体
	navMap := maps.SliceToMap(DataList,
		func(opt Data) int { return opt.ID },
		func(opt Data) Data { return opt },
	)

	// 获取导航映射
	NavID := 4
	str := fmt.Sprintf("-------导航ID:%d,导航名称:%s-------", NavID, navMap[NavID].Name)
	fmt.Println(str) // 输出：-------导航ID:4,导航名称:热门推荐-------

	// 测试2，存入结构体中的字段
	builder2 := maps.NewMapBuilder[int, string]()
	for _, data := range DataList {
		builder2.Put(data.ID, data.Name)
	}
	navMap2 := builder2.Build()

	// 获取导航映射
	NavID2 := 3
	str2 := fmt.Sprintf("-------导航ID:%d,导航名称:%s-------", NavID2, navMap2[NavID2])
	fmt.Println(str2) // 输出：-------导航ID:3,导航名称:首页推荐-------
}

// 默认值
// go test -v -run TestDefaultMap  ./tests/maps_test.go
func TestDefaultMap(t *testing.T) {
	type Data struct {
		ID     int
		Name   string
		NavID  int
		Status int
	}
	DataList := []Data{
		{ID: 3, Name: "首页推荐", NavID: 1, Status: 1},
		{ID: 4, Name: "热门推荐", NavID: 2, Status: 0},
	}

	// -----构建map start-----
	builder := maps.NewMapBuilder[int, string]()
	for _, data := range DataList {
		builder.Put(data.ID, data.Name)
	}
	navMap := builder.Build()
	// -----构建map end-----

	// -----测试1，存在id的时候-----
	for _, data := range DataList {
		name := maps.GetOrDefault(navMap, data.ID, "未知导航")
		str := fmt.Sprintf("测试1，存在id的时候:%d,导航名称:%s", data.ID, name)
		fmt.Println(str)
	}

	// 测试2，使用默认值
	name2 := maps.GetOrDefault(navMap, 5, "未知导航")
	str2 := fmt.Sprintf("测试2，不存在id的时候:%d,导航名称:%s", 5, name2)
	fmt.Println(str2) // 输出：测试2，不存在id的时候:5,导航名称:未知导航

	// 结构体类型
	builder2 := maps.NewMapBuilder[int, Data]()
	// 构建map
	for _, data := range DataList {
		builder2.Put(data.ID, data)
	}
	navMap2 := builder2.Build()

	// 测试3，默认的是结构体
	for _, data := range DataList {
		nav := maps.GetOrDefault(navMap2, data.ID, Data{ID: 0, Name: "未知导航", NavID: 0, Status: 0})
		str3 := fmt.Sprintf("测试3，默认的是结构体:%d,导航名称:%s", data.ID, nav.Name)
		fmt.Println(str3) // 输出：测试3，默认的是结构体:3,导航名称:首页推荐
	}
	nav := maps.GetOrDefault(navMap2, 5, Data{ID: 0, Name: "未知导航", NavID: 0, Status: 0})
	str4 := fmt.Sprintf("测试4，不存在	id的时候:%d,导航名称:%s", 5, nav.Name)
	fmt.Println(str4) // 输出：测试4，不存在	id的时候:5,导航名称:未知导航
}

// 测试StructSliceToMap函数
// go test -v -run TestStructSliceToMap  ./tests/maps_test.go
func TestStructSliceToMap(t *testing.T) {
	type Data struct {
		ID     int
		Name   string
		NavID  int
		Status int
	}
	DataList := []Data{
		{ID: 3, Name: "首页推荐", NavID: 1, Status: 1},
		{ID: 4, Name: "热门推荐", NavID: 2, Status: 0},
	}

	navMap := maps.StructSliceToMap(DataList, func(data Data) int { return data.ID })
	fmt.Println("导航映射:", navMap) // 输出：导航映射:map[3:{3 首页推荐 1 1} 4:{4 热门推荐 2 0}]

	// 获取导航映射
	NavID := 4
	str := fmt.Sprintf("-------导航ID:%d,导航名称:%s-------", NavID, navMap[NavID].Name)
	fmt.Println(str) // 输出：-------导航ID:4,导航名称:热门推荐-------
}
