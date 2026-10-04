package model

import (
	"slices"
	"testing"
)

func TestPasswordPolicy(t *testing.T) {
	p, err := PasswordPolicy{MinLength: 10, RequireDigits: true, MinDigits: 2, RequireSpecial: true, MinSpecial: 2, RequireMixedCase: true, Letters: LettersLatin}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		password string
		want     []string
	}{
		{"Abcdef-12!", nil},
		{"Abc-12!", []string{ViolationLength}},
		{"Abcdefg-1!", []string{ViolationDigits}},
		{"Abcdefg-12", []string{ViolationSpecial}},
		{"abcdef-12!", []string{ViolationCase}},
		{"Абвгде-12!", []string{ViolationLetters}},
		{"Abcdef-12!\x01", []string{ViolationControl}},
		{"Admin-12!!", []string{ViolationUsername}},
	}
	for _, c := range cases {
		got := p.Check(c.password, "admin-12!!")
		if !slices.Equal(got, c.want) {
			t.Errorf("%q: %v, want %v", c.password, got, c.want)
		}
	}
	p.Letters = LettersCyrillic
	if v := p.Check("Пароль-12!", ""); v != nil {
		t.Errorf("cyrillic password rejected: %v", v)
	}
	p.Letters = LettersLatinCyrillic
	if v := p.Check("Пароль-Pw12!", ""); v != nil {
		t.Errorf("mixed scripts rejected: %v", v)
	}
	p.Letters = LettersAny
	if v := p.Check("Ελληνικά-12!", ""); v != nil {
		t.Errorf("greek rejected with any letters: %v", v)
	}
}

func TestPolicyNormalize(t *testing.T) {
	bad := []PasswordPolicy{
		{MinLength: 6},
		{MinLength: 200},
		{MinLength: 8, RequireDigits: true, MinDigits: 0},
		{MinLength: 8, RequireDigits: true, MinDigits: 4, RequireSpecial: true, MinSpecial: 4, RequireMixedCase: true},
		{MinLength: 8, Letters: "greek"},
	}
	for i, b := range bad {
		if _, err := b.Normalize(); err == nil {
			t.Errorf("case %d must fail: %+v", i, b)
		}
	}
	p, err := PasswordPolicy{MinLength: 8, MinDigits: 5}.Normalize()
	if err != nil || p.MinDigits != 0 || p.Letters != LettersAny {
		t.Errorf("normalized = %+v, %v", p, err)
	}
}
