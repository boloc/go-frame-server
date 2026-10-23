package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/boloc/go-frame-server/v2/pkg/frame"
	"github.com/boloc/go-frame-server/v2/pkg/frame/rediskey"
	"github.com/redis/go-redis/v9"
)

var (
	// ErrUnavailable Redis 拿不到连接。调用方按业务决定放行还是拒绝。
	ErrUnavailable = errors.New("ratelimit: redis unavailable")
	// ErrEmptyKey 限流对象为空。
	ErrEmptyKey = errors.New("ratelimit: key is empty")
)

// Result 一次限流判断的结果。
type Result struct {
	Allowed bool
	// Banned 处于 WithBan 的封禁期。
	Banned bool
	// Limit 规则容量，见 Rule.Capacity。
	Limit int
	// Remaining 这次之后还能立刻放行几次。Status 返回的是当前还能放行几次。
	Remaining int
	// RetryAfter 被拒时至少等多久再试；放行时为 0。
	RetryAfter time.Duration
	// ResetAfter 多久之后额度完全恢复。
	ResetAfter time.Duration
}

// Limiter 在 Redis 上执行限流规则。判断和扣减在一段 Lua 里原子完成，时间取 Redis 服务端的
// TIME，多副本之间不受各自机器时钟误差影响。
type Limiter struct {
	client func() (redis.Cmdable, bool)
}

// New 用指定的 Redis 访问器创建 Limiter，测试或非默认 Redis 时用。
func New(client func() (redis.Cmdable, bool)) *Limiter {
	return &Limiter{client: client}
}

var defaultLimiter = New(frame.TryGetRedisCmdable)

// Default 使用框架默认 Redis 的 Limiter。
func Default() *Limiter { return defaultLimiter }

// Allow 用默认 Limiter 消耗 1 次额度。
func Allow(ctx context.Context, rule Rule, key string) (Result, error) {
	return defaultLimiter.Allow(ctx, rule, key)
}

// AllowN 用默认 Limiter 消耗 n 次额度。
func AllowN(ctx context.Context, rule Rule, key string, n int) (Result, error) {
	return defaultLimiter.AllowN(ctx, rule, key, n)
}

// Status 用默认 Limiter 查看额度，不消耗。
func Status(ctx context.Context, rule Rule, key string) (Result, error) {
	return defaultLimiter.Status(ctx, rule, key)
}

// Reset 用默认 Limiter 清空计数、封禁和封禁记录。
func Reset(ctx context.Context, rule Rule, key string) error {
	return defaultLimiter.Reset(ctx, rule, key)
}

// Allow 消耗 1 次额度。
func (l *Limiter) Allow(ctx context.Context, rule Rule, key string) (Result, error) {
	return l.run(ctx, rule, key, 1, false)
}

// AllowN 一次消耗 n 次额度，例如批量发送按条数计。n 超过 Rule.Capacity 时永远放不行，直接返回错误。
func (l *Limiter) AllowN(ctx context.Context, rule Rule, key string, n int) (Result, error) {
	if n < 1 {
		return Result{}, fmt.Errorf("ratelimit: n 至少为 1，当前 %d", n)
	}
	if n > rule.Capacity() {
		return Result{}, fmt.Errorf("ratelimit: n=%d 超过规则 %q 的容量 %d，永远不会放行", n, rule.name, rule.Capacity())
	}
	return l.run(ctx, rule, key, n, false)
}

// Status 只看不扣：Allowed 表示现在再来 1 次能不能放行，封禁中会返回 Banned。
// 不记被拒次数，也不会触发封禁。
//
// 配合 Allow、Reset 可以只给失败计数，例如登录：先 Status 看 Banned 是否在封禁中；
// 密码错了再 Allow 记一次失败，超限就封禁；登录成功就 Reset。
func (l *Limiter) Status(ctx context.Context, rule Rule, key string) (Result, error) {
	return l.run(ctx, rule, key, 1, true)
}

// Reset 清空这个限流对象在该规则下的计数、封禁、被拒次数和封过几次。
func (l *Limiter) Reset(ctx context.Context, rule Rule, key string) error {
	rule.mustValid()
	if key == "" {
		return ErrEmptyKey
	}
	client, ok := l.client()
	if !ok {
		return ErrUnavailable
	}
	return client.Del(ctx, rule.redisKeys(key)...).Err()
}

func (l *Limiter) run(ctx context.Context, rule Rule, key string, cost int, dry bool) (Result, error) {
	rule.mustValid()
	if key == "" {
		return Result{}, ErrEmptyKey
	}
	client, ok := l.client()
	if !ok {
		return Result{}, ErrUnavailable
	}

	dryArg := "0"
	if dry {
		dryArg = "1"
	}
	steps := make([]string, len(rule.ban.Durations))
	for i, d := range rule.ban.Durations {
		steps[i] = strconv.FormatInt(d.Milliseconds(), 10)
	}
	args := []any{
		dryArg,
		strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatUint(rand.Uint64(), 36),
		cost,
		rule.ban.Strikes,
		rule.ban.Within.Milliseconds(),
		rule.ban.Forget.Milliseconds(),
		strings.Join(steps, ","),
		rule.limit,
		rule.period.Milliseconds(),
		rule.burst,
	}

	var script *redis.Script
	switch rule.algo {
	case algoSmooth:
		script = smoothScript
	case algoSliding:
		script = slidingScript
	case algoFixed:
		script = fixedScript
	}
	vals, err := script.Run(ctx, client, rule.redisKeys(key), args...).Int64Slice()
	if err != nil {
		return Result{}, fmt.Errorf("ratelimit: 规则 %q 执行失败: %w", rule.name, err)
	}
	if len(vals) != 5 {
		return Result{}, fmt.Errorf("ratelimit: 规则 %q 返回了 %d 个值，期望 5 个", rule.name, len(vals))
	}
	return Result{
		Allowed:    vals[0] == 1,
		Banned:     vals[1] == 1,
		Limit:      rule.Capacity(),
		Remaining:  int(vals[2]),
		RetryAfter: time.Duration(vals[3]) * time.Millisecond,
		ResetAfter: time.Duration(vals[4]) * time.Millisecond,
	}, nil
}

// redisKeys 返回计数、封禁、被拒次数、封禁次数四个 key。四个 key 共用同一个
// {规则名:限流对象} hash tag，Redis Cluster 下落在同一个槽，一段 Lua 里能同时读写。
//
//	Namespace() + "ratelimit:{" + 规则名 + ":" + 限流对象 + "}:gcra"     Smooth 计数
//	Namespace() + "ratelimit:{" + 规则名 + ":" + 限流对象 + "}:window"   Sliding 计数
//	Namespace() + "ratelimit:{" + 规则名 + ":" + 限流对象 + "}:count"    Fixed 计数
//	Namespace() + "ratelimit:{" + 规则名 + ":" + 限流对象 + "}:ban"      封禁中
//	Namespace() + "ratelimit:{" + 规则名 + ":" + 限流对象 + "}:strikes"  Within 内被拒了几次
//	Namespace() + "ratelimit:{" + 规则名 + ":" + 限流对象 + "}:level"    Forget 内封过几次
func (r Rule) redisKeys(key string) []string {
	base := rediskey.Prefix(rediskey.SegRateLimit) + "{" + r.name + ":" + key + "}"
	counter := base + ":window"
	switch r.algo {
	case algoSmooth:
		counter = base + ":gcra"
	case algoFixed:
		counter = base + ":count"
	}
	return []string{counter, base + ":ban", base + ":strikes", base + ":level"}
}

// 三段脚本共用 banPrelude，都返回 {allowed, banned, remaining, retry_ms, reset_ms}。
// Run 先走 EVALSHA，遇到 NOSCRIPT 再退回 EVAL。
//
// KEYS: 计数, 封禁, 被拒次数, 封禁次数
// ARGV: dry, member, cost, strikes, within_ms, forget_ms, 各档封禁毫秒（逗号分隔）, limit, period_ms, burst
const banPrelude = `
if redis.replicate_commands then pcall(redis.replicate_commands) end

local dry = ARGV[1] == "1"
local member = ARGV[2]
local cost = tonumber(ARGV[3])
local strikes = tonumber(ARGV[4])
local within = tonumber(ARGV[5])
local forget = tonumber(ARGV[6])
local limit = tonumber(ARGV[8])
local period = tonumber(ARGV[9])
local burst = tonumber(ARGV[10])

local t = redis.call("TIME")
local sec = tonumber(t[1])
local usec = tonumber(t[2])
local now_ms = sec * 1000 + math.floor(usec / 1000)

if strikes > 0 then
	local left = redis.call("PTTL", KEYS[2])
	if left > 0 then
		return {0, 1, 0, left, left}
	end
end

-- 被拒时调用：记一次被拒，Within 内够 Strikes 次就按封过的次数选一档封禁。
-- 返回封禁毫秒数，没有封禁返回 0。Status（dry）只看不记。
local function strike()
	if strikes == 0 or dry then
		return 0
	end
	if strikes > 1 then
		local n = redis.call("INCR", KEYS[3])
		if n == 1 then
			redis.call("PEXPIRE", KEYS[3], within)
		end
		if n < strikes then
			return 0
		end
		redis.call("DEL", KEYS[3])
	end
	local steps = {}
	for s in string.gmatch(ARGV[7], "%d+") do
		steps[#steps + 1] = tonumber(s)
	end
	local level = redis.call("INCR", KEYS[4])
	redis.call("PEXPIRE", KEYS[4], forget)
	local d = steps[math.min(level, #steps)]
	redis.call("SET", KEYS[2], "1", "PX", d)
	return d
end
`

// smoothScript 是 GCRA：记录"理论到达时间" tat。每次请求把 tat 往后推 cost 个发放间隔，
// 推完之后超出 now + 容量 就拒绝。tat 存的是 Unix 毫秒时间戳，带 3 位小数，意思是额度在这一刻补满。
var smoothScript = redis.NewScript(banPrelude + `
local eps = 0.001
local now = sec * 1000 + usec / 1000
local interval = period / limit
local capacity = interval * burst

local tat = now
local raw = redis.call("GET", KEYS[1])
if raw then
	tat = math.max(tonumber(raw), now)
end

local new_tat = tat + interval * cost
local allow_at = new_tat - capacity

if allow_at - now > eps then
	local banned = strike()
	if banned > 0 then
		return {0, 1, 0, banned, banned}
	end
	local remaining = math.floor((now - (tat - capacity)) / interval + eps)
	if remaining < 0 then remaining = 0 end
	return {0, 0, remaining, math.ceil(allow_at - now), math.ceil(tat - now)}
end

local remaining = math.floor((now - allow_at) / interval + eps)
if dry then
	return {1, 0, remaining + cost, 0, math.ceil(tat - now)}
end
redis.call("SET", KEYS[1], string.format("%.3f", new_tat), "PX", math.max(1, math.ceil(new_tat - now)))
return {1, 0, remaining, 0, math.ceil(new_tat - now)}
`)

// slidingScript 用有序集合记每次放行的毫秒时间戳，先删掉窗口外的，再数窗口内还有几条。
// 被拒时，等到第 (count + cost - limit) 早的那条移出窗口就能再放行。
var slidingScript = redis.NewScript(banPrelude + `
redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", now_ms - period)
local count = redis.call("ZCARD", KEYS[1])

local function reset_after()
	local newest = redis.call("ZRANGE", KEYS[1], -1, -1, "WITHSCORES")
	if newest[2] then
		return tonumber(newest[2]) + period - now_ms
	end
	return 0
end

if count + cost > limit then
	local banned = strike()
	if banned > 0 then
		return {0, 1, 0, banned, banned}
	end
	local idx = count + cost - limit - 1
	local nth = redis.call("ZRANGE", KEYS[1], idx, idx, "WITHSCORES")
	local retry = period
	if nth[2] then
		retry = tonumber(nth[2]) + period - now_ms
	end
	local remaining = limit - count
	if remaining < 0 then remaining = 0 end
	return {0, 0, remaining, retry, reset_after()}
end

if dry then
	return {1, 0, limit - count, 0, reset_after()}
end

for i = 1, cost do
	redis.call("ZADD", KEYS[1], now_ms, member .. ":" .. i)
end
redis.call("PEXPIRE", KEYS[1], period)
return {1, 0, limit - count - cost, 0, period}
`)

// fixedScript 一个累加的计数。第一次请求时设过期时间 period，到期 key 自动删除，下一轮从 0 算。
// 被拒时要等这一轮结束，Retry-After 就是 key 的剩余过期时间。
var fixedScript = redis.NewScript(banPrelude + `
local count = tonumber(redis.call("GET", KEYS[1]) or "0")
local left = redis.call("PTTL", KEYS[1])
if left < 0 then
	left = period
end

if count + cost > limit then
	local banned = strike()
	if banned > 0 then
		return {0, 1, 0, banned, banned}
	end
	local remaining = limit - count
	if remaining < 0 then remaining = 0 end
	return {0, 0, remaining, left, left}
end

if dry then
	local reset = 0
	if count > 0 then reset = left end
	return {1, 0, limit - count, 0, reset}
end

local after = redis.call("INCRBY", KEYS[1], cost)
if after == cost then
	redis.call("PEXPIRE", KEYS[1], period)
	left = period
end
return {1, 0, limit - after, 0, left}
`)
