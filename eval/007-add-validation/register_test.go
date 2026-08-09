package user

import "testing"

func TestRegister_Valid(t *testing.T) {
	err := Register(RegisterRequest{Name: "Alice", Email: "alice@example.com", Password: "secret123"})
	if err != nil {
		t.Fatalf("valid request returned error: %v", err)
	}
}

func TestRegister_EmptyName(t *testing.T) {
	err := Register(RegisterRequest{Name: "", Email: "alice@example.com", Password: "secret123"})
	if err != ErrEmptyName {
		t.Errorf("empty name: got %v, want ErrEmptyName", err)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	cases := []string{"notanemail", "nodot@nodot", "@nodomain.com", "spaces in@email.com"}
	for _, email := range cases {
		err := Register(RegisterRequest{Name: "Alice", Email: email, Password: "secret123"})
		if err != ErrInvalidEmail {
			t.Errorf("email %q: got %v, want ErrInvalidEmail", email, err)
		}
	}
}

func TestRegister_WeakPassword(t *testing.T) {
	err := Register(RegisterRequest{Name: "Alice", Email: "alice@example.com", Password: "short"})
	if err != ErrWeakPassword {
		t.Errorf("weak password: got %v, want ErrWeakPassword", err)
	}
}

func TestRegister_ValidationOrder(t *testing.T) {
	// Empty name takes priority over invalid email.
	err := Register(RegisterRequest{Name: "", Email: "bad", Password: "secret123"})
	if err != ErrEmptyName {
		t.Errorf("empty name + bad email: got %v, want ErrEmptyName", err)
	}
}
