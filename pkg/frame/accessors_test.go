package frame

import "testing"

// TestTryAccessorsDoNotPanicWhenNotInitialized 验证未初始化时 Try 系列访问器返回 (nil, false)，不 panic。
func TestTryAccessorsDoNotPanicWhenNotInitialized(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Try 系列访问器不应该 panic，got: %v", r)
		}
	}()

	TryDefaultDB()
	TryDefaultSlaveDB()
	TryMasterDB("does-not-exist-" + t.Name())
	TrySlaveDB("does-not-exist-" + t.Name())
	TryGetRedis()
	TryGetRedisCluster()
	TryGetRedisSentinel()
	TryGetRedisCmdable()
}

// TestMustAccessorsPanicWhenNotInitialized 验证未注册时 Must 系列访问器仍会 panic。
func TestMustAccessorsPanicWhenNotInitialized(t *testing.T) {
	cases := map[string]func(){
		"MasterDB":        func() { MasterDB("does-not-exist-" + t.Name()) },
		"SlaveDB":         func() { SlaveDB("does-not-exist-" + t.Name()) },
		"GetRedisCmdable": func() { GetRedisCmdable() },
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatalf("%s 未注册时应该 panic", name)
				}
			}()
			fn()
		})
	}
}
