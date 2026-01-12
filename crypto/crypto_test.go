package crypto

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKey(t *testing.T) {
	key1, err := GenerateKey()
	require.NoError(t, err)
	assert.Len(t, key1, KeySize)

	key2, err := GenerateKey()
	require.NoError(t, err)
	assert.False(t, bytes.Equal(key1, key2), "keys should be unique")
}

func TestEncryptDecrypt(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	plaintext := []byte("hello world")
	ciphertext, err := Encrypt(plaintext, key)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, ciphertext)

	decrypted, err := Decrypt(ciphertext, key)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptDecrypt_LargePayload(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	plaintext := make([]byte, 1024*1024)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}

	ciphertext, err := Encrypt(plaintext, key)
	require.NoError(t, err)

	decrypted, err := Decrypt(ciphertext, key)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncrypt_InvalidKeySize(t *testing.T) {
	_, err := Encrypt([]byte("test"), []byte("short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key size")
}

func TestDecrypt_InvalidKeySize(t *testing.T) {
	_, err := Decrypt([]byte("test"), []byte("short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key size")
}

func TestDecrypt_CiphertextTooShort(t *testing.T) {
	key, _ := GenerateKey()
	_, err := Decrypt([]byte("short"), key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ciphertext too short")
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1, _ := GenerateKey()
	key2, _ := GenerateKey()

	plaintext := []byte("secret data")
	ciphertext, err := Encrypt(plaintext, key1)
	require.NoError(t, err)

	_, err = Decrypt(ciphertext, key2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt:")
}

func TestEncodeDecodeKey(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	encoded := EncodeKey(key)
	assert.NotEmpty(t, encoded)

	decoded, err := DecodeKey(encoded)
	require.NoError(t, err)
	assert.Equal(t, key, decoded)
}

func TestDecodeKey_Invalid(t *testing.T) {
	_, err := DecodeKey("invalid!base64")
	assert.Error(t, err)
}

func TestDecodeKey_WrongSize(t *testing.T) {
	_, err := DecodeKey("YWJjZA==") // "abcd" = 4 bytes
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key size")
}

func TestEncryptProducesUniqueCiphertext(t *testing.T) {
	key, _ := GenerateKey()
	plaintext := []byte("same input")

	ct1, _ := Encrypt(plaintext, key)
	ct2, _ := Encrypt(plaintext, key)

	assert.False(t, bytes.Equal(ct1, ct2), "ciphertexts should differ due to random nonce")
}
