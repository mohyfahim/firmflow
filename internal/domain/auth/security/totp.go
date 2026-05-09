package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
)

type TOTPManager struct {
	issuer string
	key    []byte
}

func NewTOTPManager(issuer string, key string) *TOTPManager {
	return &TOTPManager{issuer: issuer, key: []byte(key)}
}

func (t *TOTPManager) Generate(email string) (secret, otpURL, qrDataURL string, err error) {
	k, err := totp.Generate(totp.GenerateOpts{
		Issuer:      t.issuer,
		AccountName: email,
	})
	if err != nil {
		return "", "", "", err
	}
	png, err := qrcode.Encode(k.URL(), qrcode.Medium, 256)
	if err != nil {
		return "", "", "", err
	}
	return k.Secret(), k.URL(), "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

func (t *TOTPManager) Verify(secret, code string) bool {
	return totp.Validate(code, secret)
}

func (t *TOTPManager) Encrypt(secret string) (string, error) {
	block, err := aes.NewCipher(t.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(secret), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (t *TOTPManager) Decrypt(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(t.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return "", fmt.Errorf("invalid ciphertext")
	}
	plain, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func NewRecoveryCodes(count int) ([]string, error) {
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		token, err := GenerateSecureToken(10)
		if err != nil {
			return nil, err
		}
		out = append(out, token)
	}
	return out, nil
}

func BuildOTPKey(url string) (*otp.Key, error) {
	return otp.NewKeyFromURL(url)
}
