package oauth

import (
	"bytes"
	"testing"
)

func TestNewTokenEncryptor(t *testing.T) {
	tests := []struct {
		name    string
		keyLen  int
		wantErr bool
	}{
		{
			name:    "valid 32-byte key",
			keyLen:  32,
			wantErr: false,
		},
		{
			name:    "key too short",
			keyLen:  16,
			wantErr: true,
		},
		{
			name:    "key too long",
			keyLen:  64,
			wantErr: true,
		},
		{
			name:    "empty key",
			keyLen:  0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keyLen)
			for i := range key {
				key[i] = byte(i)
			}

			_, err := NewTokenEncryptor(key)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTokenEncryptor() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", err)
	}

	encryptor, err := NewTokenEncryptor(key)
	if err != nil {
		t.Fatalf("NewTokenEncryptor() error = %v", err)
	}

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{
			name:      "simple text",
			plaintext: []byte("Hello, World!"),
		},
		{
			name:      "json data",
			plaintext: []byte(`{"access_token":"abc123","refresh_token":"xyz789"}`),
		},
		{
			name:      "empty data",
			plaintext: []byte{},
		},
		{
			name:      "binary data",
			plaintext: []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd},
		},
		{
			name:      "large data",
			plaintext: bytes.Repeat([]byte("abcdefghij"), 1000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt.
			ciphertext, err := encryptor.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			// Ciphertext should be different from plaintext.
			if bytes.Equal(ciphertext, tt.plaintext) && len(tt.plaintext) > 0 {
				t.Error("ciphertext equals plaintext")
			}

			// Decrypt.
			decrypted, err := encryptor.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			// Decrypted should match original.
			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("Decrypt() = %v, want %v", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptProducesUniqueCiphertext(t *testing.T) {
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", err)
	}

	encryptor, err := NewTokenEncryptor(key)
	if err != nil {
		t.Fatalf("NewTokenEncryptor() error = %v", err)
	}

	plaintext := []byte("same plaintext")

	// Encrypt the same plaintext multiple times.
	ciphertext1, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() 1 error = %v", err)
	}

	ciphertext2, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() 2 error = %v", err)
	}

	// Due to random nonce, ciphertexts should be different.
	if bytes.Equal(ciphertext1, ciphertext2) {
		t.Error("encrypting same plaintext produced identical ciphertexts")
	}

	// But both should decrypt to the same plaintext.
	decrypted1, err := encryptor.Decrypt(ciphertext1)
	if err != nil {
		t.Fatalf("Decrypt() 1 error = %v", err)
	}

	decrypted2, err := encryptor.Decrypt(ciphertext2)
	if err != nil {
		t.Fatalf("Decrypt() 2 error = %v", err)
	}

	if !bytes.Equal(decrypted1, decrypted2) {
		t.Error("different ciphertexts decrypted to different plaintexts")
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	key1, _ := GenerateEncryptionKey()
	key2, _ := GenerateEncryptionKey()

	encryptor1, _ := NewTokenEncryptor(key1)
	encryptor2, _ := NewTokenEncryptor(key2)

	plaintext := []byte("secret data")
	ciphertext, err := encryptor1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Decrypting with wrong key should fail.
	_, err = encryptor2.Decrypt(ciphertext)
	if err == nil {
		t.Error("Decrypt() with wrong key should fail")
	}
}

func TestDecryptTamperedData(t *testing.T) {
	key, _ := GenerateEncryptionKey()
	encryptor, _ := NewTokenEncryptor(key)

	plaintext := []byte("secret data")
	ciphertext, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Tamper with the ciphertext.
	if len(ciphertext) > 0 {
		ciphertext[len(ciphertext)-1] ^= 0xff
	}

	// Decrypting tampered data should fail.
	_, err = encryptor.Decrypt(ciphertext)
	if err == nil {
		t.Error("Decrypt() with tampered data should fail")
	}
}

func TestDecryptTooShortData(t *testing.T) {
	key, _ := GenerateEncryptionKey()
	encryptor, _ := NewTokenEncryptor(key)

	// Data shorter than nonce should fail.
	_, err := encryptor.Decrypt([]byte{1, 2, 3})
	if err == nil {
		t.Error("Decrypt() with too short data should fail")
	}
}

func TestEncryptDecryptString(t *testing.T) {
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", err)
	}

	encryptor, err := NewTokenEncryptor(key)
	if err != nil {
		t.Fatalf("NewTokenEncryptor() error = %v", err)
	}

	plaintext := "Hello, this is a secret message!"

	encrypted, err := encryptor.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	// Encrypted string should be base64 encoded.
	if encrypted == plaintext {
		t.Error("encrypted string equals plaintext")
	}

	decrypted, err := encryptor.DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("DecryptString() = %s, want %s", decrypted, plaintext)
	}
}

func TestDecryptStringInvalidBase64(t *testing.T) {
	key, _ := GenerateEncryptionKey()
	encryptor, _ := NewTokenEncryptor(key)

	_, err := encryptor.DecryptString("not-valid-base64!!!")
	if err == nil {
		t.Error("DecryptString() with invalid base64 should fail")
	}
}

func TestGenerateEncryptionKey(t *testing.T) {
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", err)
	}

	if len(key) != 32 {
		t.Errorf("GenerateEncryptionKey() length = %d, want 32", len(key))
	}

	// Keys should be unique.
	key2, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey() 2 error = %v", err)
	}

	if bytes.Equal(key, key2) {
		t.Error("GenerateEncryptionKey() returned duplicate keys")
	}
}

func TestEncryptionKeyFromBase64(t *testing.T) {
	// Generate a key and encode it.
	originalKey, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", err)
	}

	encoded := EncryptionKeyToBase64(originalKey)

	// Decode it back.
	decodedKey, err := EncryptionKeyFromBase64(encoded)
	if err != nil {
		t.Fatalf("EncryptionKeyFromBase64() error = %v", err)
	}

	if !bytes.Equal(originalKey, decodedKey) {
		t.Error("decoded key doesn't match original")
	}
}

func TestEncryptionKeyFromBase64_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
	}{
		{
			name:    "invalid base64",
			encoded: "not-valid-base64!!!",
		},
		{
			name:    "wrong length",
			encoded: "YWJjZGVmZ2hpamtsbW5vcA==", // 16 bytes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EncryptionKeyFromBase64(tt.encoded)
			if err == nil {
				t.Error("EncryptionKeyFromBase64() expected error")
			}
		})
	}
}

func BenchmarkEncrypt(b *testing.B) {
	key, _ := GenerateEncryptionKey()
	encryptor, _ := NewTokenEncryptor(key)
	plaintext := []byte(`{"access_token":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9","refresh_token":"1//0g-abc123","token_type":"Bearer","expires_in":3600}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = encryptor.Encrypt(plaintext)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	key, _ := GenerateEncryptionKey()
	encryptor, _ := NewTokenEncryptor(key)
	plaintext := []byte(`{"access_token":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9","refresh_token":"1//0g-abc123","token_type":"Bearer","expires_in":3600}`)
	ciphertext, _ := encryptor.Encrypt(plaintext)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = encryptor.Decrypt(ciphertext)
	}
}
