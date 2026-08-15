// Package validate 在 handler 绑定请求后、调用 logic 前做校验。
// 支持 struct tag（go-playground/validator）与 Validatable 接口；不依赖 gin/HTTP。
package validate

import (
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/go-playground/validator/v10"
)

// Validatable 由请求 DTO 实现，表达 struct tag 表达不了的业务规则。
// 返回的 error 由 webx.Bind 转为 *errs.Error；若已是 *errs.Error 则原样透传。
type Validatable interface {
	Validate() error
}

var (
	once     sync.Once
	instance *validator.Validate
)

// Validator 返回全局共享的 validator 实例（并发安全）。自定义规则在 bootstrap 中 RegisterValidation。
func Validator() *validator.Validate {
	once.Do(func() {
		instance = validator.New(validator.WithRequiredStructEnabled())
	})
	return instance
}

// Struct 对结构体做 tag 校验；通过返回 nil，否则返回 field -> 说明。
func Struct(v any) map[string]string {
	err := Validator().Struct(v)
	if err == nil {
		return nil
	}

	fieldErrs, ok := err.(validator.ValidationErrors)
	if !ok {
		// 非字段级错误（例如传入非结构体）时整体返回。
		return map[string]string{"_error": err.Error()}
	}

	out := make(map[string]string, len(fieldErrs))
	for _, fe := range fieldErrs {
		out[toSnakeCase(fe.Field())] = describe(fe)
	}
	return out
}

// describe 把 FieldError 转成可读说明；未覆盖的 tag 走默认文案。
func describe(fe validator.FieldError) string {
	field := toSnakeCase(fe.Field())
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s 不能为空", field)
	case "min":
		return fmt.Sprintf("%s 不能小于 %s", field, fe.Param())
	case "max":
		return fmt.Sprintf("%s 不能大于 %s", field, fe.Param())
	case "len":
		return fmt.Sprintf("%s 长度必须为 %s", field, fe.Param())
	case "email":
		return fmt.Sprintf("%s 不是合法的邮箱地址", field)
	case "oneof":
		return fmt.Sprintf("%s 必须是以下取值之一: %s", field, fe.Param())
	case "gte":
		return fmt.Sprintf("%s 不能小于 %s", field, fe.Param())
	case "lte":
		return fmt.Sprintf("%s 不能大于 %s", field, fe.Param())
	default:
		return fmt.Sprintf("%s 未通过校验规则: %s", field, fe.Tag())
	}
}

// toSnakeCase 把导出字段名转为 snake_case；无法转换时原样返回。
func toSnakeCase(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 && (unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimPrefix(b.String(), "_")
}
