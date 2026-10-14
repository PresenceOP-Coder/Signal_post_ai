package orgnr

import (
	"errors"
	"strings"
	"unicode"
)

var (
	ErrEmptyOrgnr        = errors.New("orgnr is empty")
	ErrInvalidLength     = errors.New("orgnr must contain exactly 9 digits")
	ErrInvalidCharacters = errors.New("orgnr contains non-numeric characters")
	ErrInvalidChecksum   = errors.New("orgnr failed MOD11 checksum validation")
)

// mod11Weights are the official weights used for the first 8 digits of a Norwegian organisation number.
var mod11Weights = [8]int{3, 2, 7, 6, 5, 4, 3, 2}

// Normalize cleans the raw input string by:
// 1. Trimming leading and trailing whitespace.
// 2. Stripping optional case-insensitive "NO" or "NO " prefix.
// 3. Stripping optional case-insensitive "MVA" or " MVA" suffix.
// 4. Removing internal spaces, dots, and hyphens.
func Normalize(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	// Case-insensitive removal of NO prefix
	upper := strings.ToUpper(s)
	if strings.HasPrefix(upper, "NO") {
		s = strings.TrimSpace(s[2:])
	}

	// Case-insensitive removal of MVA suffix
	upper = strings.ToUpper(s)
	if strings.HasSuffix(upper, "MVA") {
		s = strings.TrimSpace(s[:len(s)-3])
	}

	// Remove common separating punctuation (spaces, dashes, dots)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if unicode.IsSpace(r) || r == '-' || r == '.' {
			continue
		} else {
			// keep non-digit characters so length/character validation can detect invalid input
			b.WriteRune(r)
		}
	}

	return b.String()
}

// Validate checks whether the normalized organisation number is a valid 9-digit Norwegian orgnr with correct MOD11 checksum.
func Validate(normalized string) error {
	if normalized == "" {
		return ErrEmptyOrgnr
	}

	if len(normalized) != 9 {
		return ErrInvalidLength
	}

	for _, r := range normalized {
		if !unicode.IsDigit(r) {
			return ErrInvalidCharacters
		}
	}

	digits := [9]int{}
	for i, r := range normalized {
		digits[i] = int(r - '0')
	}

	sum := 0
	for i := 0; i < 8; i++ {
		sum += digits[i] * mod11Weights[i]
	}

	remainder := sum % 11
	var checkDigit int
	if remainder == 0 {
		checkDigit = 0
	} else if remainder == 1 {
		// When remainder is 1, 11 - 1 = 10 (not a single digit).
		// Such numbers cannot be valid Norwegian organisation numbers.
		return ErrInvalidChecksum
	} else {
		checkDigit = 11 - remainder
	}

	if digits[8] != checkDigit {
		return ErrInvalidChecksum
	}

	return nil
}

// CleanAndValidate normalizes and validates the organisation number in one step.
// Returns the clean 9-digit string or an error.
func CleanAndValidate(raw string) (string, error) {
	norm := Normalize(raw)
	if err := Validate(norm); err != nil {
		return norm, err
	}
	return norm, nil
}
