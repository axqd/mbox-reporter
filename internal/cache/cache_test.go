package cache

import (
	"testing"
)

func TestHasEmail(t *testing.T) {
	tr := Trashed{Emails: []string{"alice@example.com", "bob@other.org"}}

	tests := []struct {
		addr string
		want bool
	}{
		{"alice@example.com", true},
		{"ALICE@EXAMPLE.COM", true},
		{"Alice@Example.Com", true},
		{"bob@other.org", true},
		{"charlie@example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tr.HasEmail(tt.addr); got != tt.want {
			t.Errorf("HasEmail(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func TestHasEmail_Empty(t *testing.T) {
	tr := Trashed{}
	if tr.HasEmail("any@example.com") {
		t.Error("HasEmail on empty list should return false")
	}
}

func TestAddEmail(t *testing.T) {
	var tr Trashed

	tr.AddEmail("Alice@Example.com")
	if len(tr.Emails) != 1 {
		t.Fatalf("len = %d, want 1", len(tr.Emails))
	}
	if tr.Emails[0] != "alice@example.com" {
		t.Errorf("stored = %q, want lowercased", tr.Emails[0])
	}

	// Duplicate (different case) should not add.
	tr.AddEmail("ALICE@EXAMPLE.COM")
	if len(tr.Emails) != 1 {
		t.Errorf("len = %d after duplicate, want 1", len(tr.Emails))
	}

	// Different address should add.
	tr.AddEmail("bob@other.org")
	if len(tr.Emails) != 2 {
		t.Errorf("len = %d, want 2", len(tr.Emails))
	}
}

func TestExcludeSet(t *testing.T) {
	tr := Trashed{Emails: []string{"Alice@Example.com", "BOB@other.org"}}
	set := tr.ExcludeSet()

	if len(set) != 2 {
		t.Fatalf("len = %d, want 2", len(set))
	}
	if _, ok := set["alice@example.com"]; !ok {
		t.Error("expected lowercased alice in set")
	}
	if _, ok := set["bob@other.org"]; !ok {
		t.Error("expected lowercased bob in set")
	}
}

func TestExcludeSet_Empty(t *testing.T) {
	tr := Trashed{}
	if set := tr.ExcludeSet(); set != nil {
		t.Errorf("expected nil for empty trashed, got %v", set)
	}
}
