package numwords

import "testing"

func TestAmount(t *testing.T) {
	tests := []struct {
		in   string
		def  Lang
		want string
	}{
		{"1234.56 руб.", RU, "одна тысяча двести тридцать четыре рубля 56 копеек"},
		{"21 руб", RU, "двадцать один рубль"},
		{"22 рубля 05 коп", RU, "двадцать два рубля"},
		{"1000000 руб", RU, "один миллион рублей"},
		{"2 002 000,50 руб", RU, "два миллиона две тысячи рублей 50 копеек"},
		{"11 руб 01 коп", RU, "одиннадцать рублей"},
		{"0 руб", RU, "ноль рублей"},
		{"$12.50", RU, "двенадцать долларов 50 центов"},
		{"$1.01", EN, "one dollar 01 cent"},
		{"99 EUR", RU, "ninety-nine euros"},
		{"99 евро", EN, "девяносто девять евро"},
		{"€3", EN, "three euros"},
		{"1,234 USD", EN, "one thousand two hundred thirty-four dollars"},
		{"21.5 dollars", RU, "twenty-one dollars 50 cents"},
		{"-5 руб", RU, "минус пять рублей"},
		{"-5 usd", EN, "minus five dollars"},
		{"105", RU, "сто пять"},
		{"2000", RU, "две тысячи"},
		{"1000", RU, "одна тысяча"},
		{"345 000 000", EN, "three hundred forty-five million"},
		{"999 999 999 999 999", EN, "nine hundred ninety-nine trillion nine hundred ninety-nine billion nine hundred ninety-nine million nine hundred ninety-nine thousand nine hundred ninety-nine"},
		{"7 ₽", RU, "семь рублей"},
	}
	for _, tt := range tests {
		got, err := Amount(tt.in, tt.def)
		if err != nil || got != tt.want {
			t.Errorf("Amount(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestAmountErrors(t *testing.T) {
	for _, in := range []string{"", "abc", "1.234 руб", "1.5", "5 zzz", "1234567890123456"} {
		if got, err := Amount(in, RU); err == nil {
			t.Errorf("Amount(%q) = %q, want an error", in, got)
		}
	}
}

func TestWordsFeminine(t *testing.T) {
	if got := Words(2, RU, true); got != "две" {
		t.Errorf("Words(2, RU, feminine) = %q", got)
	}
	if got := Words(21, RU, true); got != "двадцать одна" {
		t.Errorf("Words(21, RU, feminine) = %q", got)
	}
	if got := Words(0, EN, false); got != "zero" {
		t.Errorf("Words(0, EN) = %q", got)
	}
}
