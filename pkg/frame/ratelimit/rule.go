package ratelimit

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

type algorithm uint8

const (
	algoSmooth  algorithm = iota + 1 // 平滑限流(等价于令牌桶)
	algoSliding                      // 滑动窗口(逐条记时间)
	algoFixed                        // 固定窗口(一个计数累加)
)

// Rule 一条限流规则。只能用 Smooth / Sliding / Fixed 构造，零值不可用。
//
// name 会拼进 Redis key：同名规则、同一个限流对象共用一份额度，不同名的规则互不影响。
// 所以全局一层、分组一层各起一个名字，就不会出现两层中间件打在同一个计数上。
type Rule struct {
	name   string        // 规则名，会拼进 Redis key
	algo   algorithm     // 算法
	limit  int           // 限制次数
	period time.Duration // 时间窗口
	burst  int           // 突发次数
	ban    Ban           // 封禁策略
}

// Ban 封禁策略。被拒到一定次数才封，封禁时长逐级加长。
//
//	ratelimit.Ban{
//		Strikes:   20,                                                      	// 在 Within 内被拒多少次才封禁。1 表示第一次被拒就封。
//		Within:    time.Minute, 												// 统计被拒次数的时间段(从第一次被拒开始算)
//		Durations: []time.Duration{10 * time.Minute, time.Hour, 24 * time.Hour}, // 第 1、2、3 次及以后各封多久
//		Forget:    24 * time.Hour,                                          	// 24 小时没再被封，从第 1 档重新算
//	}
//
// 正常用户被 429 后会停下来，被拒次数到不了 Strikes；
type Ban struct {
	// Strikes 在 Within 内被拒多少次才封禁。1 表示第一次被拒就封。
	Strikes int
	// Within 统计被拒次数的时间段，从第一次被拒开始算，到期清零。Strikes 为 1 时不用填。
	Within time.Duration
	// Durations 依次是第 1、2、3…次封禁的时长。封禁次数超过长度后，一直用最后一档。
	Durations []time.Duration
	// Forget 最近一次封禁之后多久没有再被封，下次就从第 1 档重新算。不能短于最长的一档。
	Forget time.Duration
}

// Smooth 平滑限流（GCRA，等价于令牌桶）。长期平均每 period 最多 limit 次，瞬时最多连续 burst 次。
//
// 额度按时间连续恢复，没有"窗口边界"：固定窗口在边界前后各打满一次，两秒内能打出 2×limit；
// 这里任意一段时长 t 内最多 burst + limit×t/period 次。burst 越小越平滑，越大越能容忍突发
// （例如 App 冷启动时并发拉一批接口）。
//
// 只存一个时间戳，内存和次数无关，适合全局兜底、按用户限速这类次数大的场景。
func Smooth(name string, limit int, period time.Duration, burst int) Rule {
	r := Rule{name: name, algo: algoSmooth, limit: limit, period: period, burst: burst}
	r.mustValid()
	return r
}

// Sliding 滑动窗口。任意连续 period 内最多 limit 次，没有突发余量。
//
// 每次放行记一条时间戳，内存随 limit 增长，适合次数少、要求"说多少就是多少"的敏感操作：
// 注册、登录失败、发短信验证码、答密保。limit 上千的场景用 Smooth。
func Sliding(name string, limit int, period time.Duration) Rule {
	r := Rule{name: name, algo: algoSliding, limit: limit, period: period}
	r.mustValid()
	return r
}

// Fixed 固定窗口。从这个对象第一次请求开始计时，period 内累计超过 limit 次就拒绝，
// 到期 key 自动删除，下一次请求重新从 0 算。
//
// Redis 里只存一个累加的数字，最省、最直观。代价是两轮交界处有空子：上一轮末尾打满、
// 新一轮开头又能马上打满，短时间内最多 2×limit。配 WithBan 时，交界处一被拒就开始累计
// 被拒次数，钻这个空子的脚本很快会被封。
func Fixed(name string, limit int, period time.Duration) Rule {
	r := Rule{name: name, algo: algoFixed, limit: limit, period: period}
	r.mustValid()
	return r
}

// WithBan 按 b 封禁持续超限的对象。
//
// 不封禁时，被拒的只是超出的那部分请求，额度一恢复就能继续打，脚本照样拿满上限。
// 封禁期间这个对象的请求全部拒绝，额度恢复了也不放行，Retry-After 是封禁剩余时间。
func (r Rule) WithBan(b Ban) Rule {
	if b.Strikes < 1 {
		panic(fmt.Sprintf("ratelimit: 规则 %q 的 Ban.Strikes 至少为 1，当前 %d", r.name, b.Strikes))
	}
	if b.Strikes > 1 && b.Within < time.Millisecond {
		panic(fmt.Sprintf("ratelimit: 规则 %q 的 Ban.Strikes 大于 1 时要填 Within", r.name))
	}
	if len(b.Durations) == 0 {
		panic(fmt.Sprintf("ratelimit: 规则 %q 的 Ban.Durations 至少要有一档", r.name))
	}
	longest := time.Duration(0)
	for _, d := range b.Durations {
		if d < time.Millisecond {
			panic(fmt.Sprintf("ratelimit: 规则 %q 的封禁时长至少 1ms，当前 %s", r.name, d))
		}
		longest = max(longest, d)
	}
	if b.Forget < longest {
		panic(fmt.Sprintf("ratelimit: 规则 %q 的 Ban.Forget 是 %s，短于最长的封禁 %s，封禁还没结束就会忘记封过几次", r.name, b.Forget, longest))
	}
	b.Durations = slices.Clone(b.Durations)
	r.ban = b
	return r
}

// Name 规则名。
func (r Rule) Name() string { return r.name }

// Capacity 一次最多能放行的次数：Smooth 是 burst，Sliding、Fixed 是 limit。
// 也是响应头 X-RateLimit-Limit 的值。
func (r Rule) Capacity() int {
	if r.algo == algoSmooth {
		return r.burst
	}
	return r.limit
}

func (r Rule) String() string {
	var s string
	switch r.algo {
	case algoSmooth:
		s = fmt.Sprintf("%s: smooth %d/%s burst %d", r.name, r.limit, r.period, r.burst)
	case algoSliding:
		s = fmt.Sprintf("%s: sliding %d/%s", r.name, r.limit, r.period)
	case algoFixed:
		s = fmt.Sprintf("%s: fixed %d/%s", r.name, r.limit, r.period)
	default:
		return "ratelimit: 未初始化的规则"
	}
	if r.banned() {
		s += fmt.Sprintf(" ban after %d strikes in %s for %v, forget after %s",
			r.ban.Strikes, r.ban.Within, r.ban.Durations, r.ban.Forget)
	}
	return s
}

func (r Rule) banned() bool { return r.ban.Strikes > 0 }

func (r Rule) mustValid() {
	switch {
	case r.algo == 0:
		panic("ratelimit: 规则未初始化，用 ratelimit.Smooth、Sliding 或 Fixed 构造")
	case strings.TrimSpace(r.name) == "":
		panic("ratelimit: 规则名不能为空，它决定 Redis key，同名规则共用额度")
	case strings.ContainsAny(r.name, "{}"):
		panic(fmt.Sprintf("ratelimit: 规则名 %q 不能包含花括号，花括号用作 Redis Cluster 的 hash tag", r.name))
	case r.limit < 1:
		panic(fmt.Sprintf("ratelimit: 规则 %q 的 limit 至少为 1，当前 %d", r.name, r.limit))
	case r.period < time.Millisecond:
		panic(fmt.Sprintf("ratelimit: 规则 %q 的 period 至少 1ms，当前 %s", r.name, r.period))
	case r.algo == algoSmooth && r.burst < 1:
		panic(fmt.Sprintf("ratelimit: 规则 %q 的 burst 至少为 1，当前 %d", r.name, r.burst))
	}
}
