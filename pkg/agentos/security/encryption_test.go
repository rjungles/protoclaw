package security

import (
	"bytes"
	"testing"
)

func TestNewEncryptor(t *testing.T) {
	masterKey := []byte("my-master-key-32-bytes-long!!")
	encryptor := NewEncryptor(masterKey)

	if encryptor == nil {
		t.Fatal("NewEncryptor() returned nil")
	}

	if len(encryptor.key) != 32 {
		t.Errorf("key length = %d, want 32", len(encryptor.key))
	}
}

func TestEncryptor_EncryptDecrypt(t *testing.T) {
	masterKey := []byte("my-master-key-32-bytes-long!!")
	encryptor := NewEncryptor(masterKey)

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"simple text", []byte("hello world")},
		{"empty", []byte("")},
		{"long text", bytes.Repeat([]byte("a"), 10000)},
		{"binary data", []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd}},
		{"unicode", []byte("Hello, 世界! Привет, мир! 🌍")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.plaintext) == 0 {
				// Empty plaintext should error
				_, err := encryptor.Encrypt(tt.plaintext)
				if err == nil {
					t.Error("Encrypt should error for empty plaintext")
				}
				return
			}

			ciphertext, err := encryptor.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			if ciphertext == "" {
				t.Error("Encrypt() returned empty ciphertext")
			}

			decrypted, err := encryptor.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("Decrypt() = %v, want %v", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptor_DifferentPlaintexts(t *testing.T) {
	masterKey := []byte("my-master-key-32-bytes-long!!")
	encryptor := NewEncryptor(masterKey)

	// Encrypt same plaintext twice should produce different ciphertexts
	plaintext := []byte("test message")

	ciphertext1, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	ciphertext2, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	if ciphertext1 == ciphertext2 {
		t.Error("Same plaintext should produce different ciphertexts (due to random nonce)")
	}

	// But both should decrypt to the same plaintext
	decrypted1, _ := encryptor.Decrypt(ciphertext1)
	decrypted2, _ := encryptor.Decrypt(ciphertext2)

	if !bytes.Equal(decrypted1, plaintext) || !bytes.Equal(decrypted2, plaintext) {
		t.Error("Both ciphertexts should decrypt to same plaintext")
	}
}

func TestEncryptor_DifferentKeys(t *testing.T) {
	key1 := []byte("key-1-32-bytes-long-for-aes!!")
	key2 := []byte("key-2-32-bytes-long-for-aes!!")

	encryptor1 := NewEncryptor(key1)
	encryptor2 := NewEncryptor(key2)

	plaintext := []byte("secret message")

	ciphertext, _ := encryptor1.Encrypt(plaintext)

	// Try to decrypt with different key
	_, err := encryptor2.Decrypt(ciphertext)
	if err == nil {
		t.Error("Decrypt with wrong key should fail")
	}
}

func TestEncryptor_TamperedCiphertext(t *testing.T) {
	masterKey := []byte("my-master-key-32-bytes-long!!")
	encryptor := NewEncryptor(masterKey)

	plaintext := []byte("sensitive data")
	ciphertext, _ := encryptor.Encrypt(plaintext)

	// Tamper with ciphertext
	tampered := ciphertext[:len(ciphertext)-1] + "X"

	_, err := encryptor.Decrypt(tampered)
	if err == nil {
		t.Error("Decrypt should fail for tampered ciphertext")
	}
}

func TestEncryptor_EmptyCiphertext(t *testing.T) {
	masterKey := []byte("my-master-key-32-bytes-long!!")
	encryptor := NewEncryptor(masterKey)

	_, err := encryptor.Decrypt("")
	if err == nil {
		t.Error("Decrypt should fail for empty ciphertext")
	}
}

func TestEncryptor_InvalidBase64(t *testing.T) {
	masterKey := []byte("my-master-key-32-bytes-long!!")
	encryptor := NewEncryptor(masterKey)

	_, err := encryptor.Decrypt("not-valid-base64!!!")
	if err == nil {
		t.Error("Decrypt should fail for invalid base64")
	}
}

func TestEncryptor_ShortCiphertext(t *testing.T) {
	masterKey := []byte("my-master-key-32-bytes-long!!")
	encryptor := NewEncryptor(masterKey)

	// Too short to contain nonce + ciphertext
	_, err := encryptor.Decrypt("YWJj") // "abc" in base64
	if err == nil {
		t.Error("Decrypt should fail for too short ciphertext")
	}
}

func TestGenerateKey(t *testing.T) {
	key1, err := GenerateKey(32)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	if len(key1) != 32 {
		t.Errorf("key length = %d, want 32", len(key1))
	}

	key2, _ := GenerateKey(32)
	if bytes.Equal(key1, key2) {
		t.Error("GenerateKey should produce different keys")
	}
}

func TestDeriveKey(t *testing.T) {
	password := "my-password"
	salt := []byte("random-salt")

	key1 := DeriveKey(password, salt)
	key2 := DeriveKey(password, salt)

	if !bytes.Equal(key1, key2) {
		t.Error("Same password and salt should produce same key")
	}

	// Different salt should produce different key
	differentSalt := []byte("different-salt")
	key3 := DeriveKey(password, differentSalt)
	if bytes.Equal(key1, key3) {
		t.Error("Different salt should produce different key")
	}

	// Different password should produce different key
	differentPassword := "different-password"
	key4 := DeriveKey(differentPassword, salt)
	if bytes.Equal(key1, key4) {
		t.Error("Different password should produce different key")
	}
}

func BenchmarkEncrypt(b *testing.B) {
	masterKey := []byte("my-master-key-32-bytes-long!!")
	encryptor := NewEncryptor(masterKey)
	plaintext := []byte("benchmark plaintext data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encryptor.Encrypt(plaintext)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	masterKey := []byte("my-master-key-32-bytes-long!!")
	encryptor := NewEncryptor(masterKey)
	plaintext := []byte("benchmark plaintext data")
	ciphertext, _ := encryptor.Encrypt(plaintext)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encryptor.Decrypt(ciphertext)
	}
}

func BenchmarkEncryptLarge(b *testing.B) {
	masterKey := []byte("my-master-key-32-bytes-long!!")
	encryptor := NewEncryptor(masterKey)
	plaintext := bytes.Repeat([]byte("a"), 1000000) // 1MB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encryptor.Encrypt(plaintext)
	}
}
