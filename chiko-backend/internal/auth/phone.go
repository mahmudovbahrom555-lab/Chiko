package auth

import (
	"fmt"

	"github.com/nyaruka/phonenumbers"
)

// NormalizePhone принимает номер телефона и код страны-подсказки,
// возвращает E.164 формат (+998901234567).
// Не предполагает страну по умолчанию — клиент передаёт defaultRegion из UI.
func NormalizePhone(raw, defaultRegion string) (string, error) {
	if defaultRegion == "" {
		defaultRegion = "UZ" // подсказка по умолчанию, не ограничение
	}

	num, err := phonenumbers.Parse(raw, defaultRegion)
	if err != nil {
		return "", fmt.Errorf("invalid phone number: %w", err)
	}

	if !phonenumbers.IsValidNumber(num) {
		return "", fmt.Errorf("phone number is not valid")
	}

	return phonenumbers.Format(num, phonenumbers.E164), nil
}
