package security

import (
	"encoding/base64"
	"strings"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := mustKey(t)
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"short", "sk-abc123"},
		{"unicode", "密钥 🔑 emoji"},
		{"long", strings.Repeat("A", 1<<20)}, // 1 MiB
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ct, err := EncryptString(c.in, key)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			if ct == c.in && c.in != "" {
				t.Fatalf("ciphertext equal to plaintext")
			}
			pt, err := DecryptString(ct, key)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if pt != c.in {
				t.Fatalf("round-trip mismatch: want %q got %q", c.in, pt)
			}
		})
	}
}

func TestEncryptUsesFreshNonce(t *testing.T) {
	key := mustKey(t)
	a, err := EncryptString("same", key)
	if err != nil {
		t.Fatal(err)
	}
	b, err := EncryptString("same", key)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two encryptions of same plaintext produced identical ciphertext; nonce not random")
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	key := mustKey(t)
	ct, err := EncryptString("hello", key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0x01 // flip last bit of tag
	tampered := base64.StdEncoding.EncodeToString(raw)
	if _, err := DecryptString(tampered, key); err == nil {
		t.Fatal("expected error for tampered ciphertext")
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	key := mustKey(t)
	ct, err := EncryptString("hello", key)
	if err != nil {
		t.Fatal(err)
	}
	wrong := make([]byte, 32)
	for i := range wrong {
		wrong[i] = 0xFF
	}
	if _, err := DecryptString(ct, wrong); err == nil {
		t.Fatal("expected error for wrong key")
	}
}

func TestDecryptRejectsMalformedInput(t *testing.T) {
	key := mustKey(t)
	cases := []string{
		"",
		"not-base64-!!!",
		base64.StdEncoding.EncodeToString([]byte("short")), // shorter than nonce+tag
	}
	for _, c := range cases {
		if _, err := DecryptString(c, key); err == nil {
			t.Fatalf("expected error for input %q", c)
		}
	}
}

func TestEncryptRejectsBadKeyLength(t *testing.T) {
	for _, badLen := range []int{0, 1, 15, 16, 24, 31, 33, 64} {
		k := make([]byte, badLen)
		if _, err := EncryptString("x", k); err == nil {
			t.Fatalf("expected error for key len %d", badLen)
		}
	}
}

func TestLoadKeyFromEnv(t *testing.T) {
	varName := "SHARED_TEST_AESGCM_KEY"

	t.Run("missing", func(t *testing.T) {
		t.Setenv(varName, "")
		if _, err := LoadKeyFromEnv(varName); err == nil {
			t.Fatal("expected error for missing env var")
		}
	})

	t.Run("not base64", func(t *testing.T) {
		t.Setenv(varName, "!!!not-base64!!!")
		if _, err := LoadKeyFromEnv(varName); err == nil {
			t.Fatal("expected error for non-base64 value")
		}
	})

	t.Run("wrong length", func(t *testing.T) {
		t.Setenv(varName, base64.StdEncoding.EncodeToString([]byte("tooshort")))
		if _, err := LoadKeyFromEnv(varName); err == nil {
			t.Fatal("expected error for wrong-length key")
		}
	})

	t.Run("valid", func(t *testing.T) {
		want := mustKey(t)
		t.Setenv(varName, base64.StdEncoding.EncodeToString(want))
		got, err := LoadKeyFromEnv(varName)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 32 {
			t.Fatalf("got key length %d, want 32", len(got))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("key byte %d mismatch: got %d want %d", i, got[i], want[i])
			}
		}
	})
}
