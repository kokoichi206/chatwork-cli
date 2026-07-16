package releaseguard

import "testing"

func TestValidateAcceptsCurrentMainAndNewerTag(t *testing.T) {
	err := Validate("v0.3.0", "abc123", "abc123", []string{"v0.1.0", "v0.2.1"})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsTagOutsideMain(t *testing.T) {
	err := Validate("v0.3.0", "abc123", "def456", []string{"v0.2.1"})
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
}

func TestValidateRejectsOlderTag(t *testing.T) {
	err := Validate("v0.2.0", "abc123", "abc123", []string{"v0.2.1"})
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
}

func TestValidateSkipsCurrentTagAndInvalidExistingTags(t *testing.T) {
	err := Validate("v0.2.1", "abc123", "abc123", []string{"v0.2.01", "v0.2.1"})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	err = Validate("v0.2.1", "abc123", "abc123", []string{"v0.2.1-build"})
	if err != nil {
		t.Fatalf("Validate() ignored invalid existing tag error = %v", err)
	}
}

func TestParseTagRejectsInvalidTags(t *testing.T) {
	tests := []string{
		"0.1.0",
		"v0.1",
		"v0.1.0-beta.1",
		"v01.1.0",
		"v0.x.0",
	}

	for _, tt := range tests {
		_, err := ParseTag(tt)
		if err == nil {
			t.Fatalf("ParseTag(%q) error = nil", tt)
		}
	}
}
