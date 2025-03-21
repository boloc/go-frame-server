package constant

const (
	// 限流 [short_link_code] -> [ratelimit_key]
	RateLimitKey = "shortlink:ratelimit:%s:tokens"

	// 短链code位图[0,1000000] [bitmap]
	ShortLinkCodeBitmapKey = "shortlink:shortcode_bitmap:%s"

	// 短链code与长链映射关系 [short_link_code] -> [original_url]
	ShortLinkCodeForOriginalUrlKey = "shortlink:short_link_code_for_original_url:%s"
)
