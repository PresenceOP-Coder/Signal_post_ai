package orgnr

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"974760673", "974760673"},
		{" 974 760 673 ", "974760673"},
		{"NO 974 760 673 MVA", "974760673"},
		{"no923609016mva", "923609016"},
		{"NO 984.851.006", "984851006"},
		{"923-609-016", "923609016"},
		{"", ""},
		{"   ", ""},
		{"NO MVA", ""},
	}

	for _, tt := range tests {
		got := Normalize(tt.input)
		if got != tt.expected {
			t.Errorf("Normalize(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestValidate(t *testing.T) {
	valid := []string{
		"974760673", // Brønnøysundregistrene
		"923609016", // Equinor ASA
		"984851006", // DNB Bank ASA
		"979860021", // Telenor ASA
		"984661185", // Posten Norge AS
	}

	for _, nr := range valid {
		if err := Validate(nr); err != nil {
			t.Errorf("Validate(%q) unexpectedly failed: %v", nr, err)
		}
	}

	invalid := []struct {
		nr  string
		err error
	}{
		{"", ErrEmptyOrgnr},
		{"12345678", ErrInvalidLength},
		{"1234567890", ErrInvalidLength},
		{"97476067A", ErrInvalidCharacters},
		{"974760674", ErrInvalidChecksum}, // Bad check digit (was 3, now 4)
		{"923609015", ErrInvalidChecksum}, // Bad check digit (was 6, now 5)
		{"123456780", ErrInvalidChecksum}, // Check digit should be 5
	}

	for _, tt := range invalid {
		err := Validate(tt.nr)
		if err == nil {
			t.Errorf("Validate(%q) should have failed with %v, but passed", tt.nr, tt.err)
		} else if tt.err != nil && err != tt.err {
			t.Errorf("Validate(%q) error = %v; want %v", tt.nr, err, tt.err)
		}
	}
}

func TestCleanAndValidate(t *testing.T) {
	clean, err := CleanAndValidate(" NO 974 760 673 MVA ")
	if err != nil {
		t.Fatalf("CleanAndValidate failed: %v", err)
	}
	if clean != "974760673" {
		t.Errorf("CleanAndValidate returned %q; want 974760673", clean)
	}

	_, err = CleanAndValidate("NO 123 456 780 MVA")
	if err == nil {
		t.Errorf("CleanAndValidate should have failed for invalid checksum")
	}
}
