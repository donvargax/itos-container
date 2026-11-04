package yaml

import (
	"math/big"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

// decimalNumber normalizes a validated decimal spelling without computing any
// power of ten. Arbitrary precision is needed only to adjust the exponent by the
// number of fractional and trailing zero digits present in the input itself.
func decimalNumber(text string) port.Decimal {
	exponentText := "0"
	if i := strings.IndexAny(text, "eE"); i >= 0 {
		text, exponentText = text[:i], text[i+1:]
	}
	sign := ""
	if strings.HasPrefix(text, "-") {
		sign = "-"
	}
	text = strings.TrimPrefix(strings.TrimPrefix(text, "-"), "+")
	whole, fraction, _ := strings.Cut(text, ".")
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		return port.Decimal{Coefficient: "0", Exponent: "0"}
	}
	coefficient := strings.TrimRight(digits, "0")
	shift := len(digits) - len(coefficient) - len(fraction)
	exponent, _ := new(big.Int).SetString(exponentText, 10)
	exponent.Add(exponent, big.NewInt(int64(shift)))
	return port.Decimal{Coefficient: sign + coefficient, Exponent: exponent.String()}
}
