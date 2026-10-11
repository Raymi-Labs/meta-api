package setting

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

var StripeApiSecret = ""
var StripeWebhookSecret = ""
var StripePriceId = ""
var StripeUnitPrice = 8.0
var StripeMinTopUp = 1
var StripePromotionCodesEnabled = false

// Empty presentation settings preserve existing PayMethods customizations.
var StripeDisplayName = ""
var StripeDisplayIcon = ""

func ValidateStripeDisplayOption(key, value string) error {
	switch key {
	case "StripeDisplayName":
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 40 {
			return fmt.Errorf("支付显示名称不能超过 40 个字符")
		}
		for _, char := range value {
			if unicode.IsControl(char) {
				return fmt.Errorf("支付显示名称不能包含控制字符")
			}
		}
	case "StripeDisplayIcon":
		switch value {
		case "", "stripe", "wxpay", "alipay", "card":
		default:
			return fmt.Errorf("不支持的支付显示图标")
		}
	}
	return nil
}
