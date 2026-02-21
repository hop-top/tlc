package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

const (
	saltSize   = 32
	keySize    = 32 // AES-256
	iterations = 100000
)

// FileStore implements Store using an encrypted file.
type FileStore struct {
	path     string
	password string
}

type encryptedCredential struct {
	Service    string `json:"service"`
	Account    string `json:"account"`
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

type credentialFile struct {
	Credentials []encryptedCredential `json:"credentials"`
}

// NewFileStore creates a new FileStore.
func NewFileStore(path string, password string) *FileStore {
	return &FileStore{
		path:     path,
		password: password,
	}
}

func (s *FileStore) deriveKey(salt []byte) []byte {
	return pbkdf2.Key([]byte(s.password), salt, iterations, keySize, sha256.New)
}

func (s *FileStore) loadFile() (*credentialFile, error) {
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		return &credentialFile{Credentials: []encryptedCredential{}}, nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read credential file: %w", err)
	}

	var file credentialFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credential file: %w", err)
	}

	return &file, nil
}

func (s *FileStore) saveFile(file *credentialFile) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credential file: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write credential file: %w", err)
	}

	return nil
}

// Get retrieves a credential from file.
func (s *FileStore) Get(service, account string) (*Credential, error) {
	file, err := s.loadFile()
	if err != nil {
		return nil, err
	}

	for _, ec := range file.Credentials {
		if ec.Service == service && ec.Account == account {
			key := s.deriveKey(ec.Salt)
			block, err := aes.NewCipher(key)
			if err != nil {
				return nil, fmt.Errorf("failed to create cipher: %w", err)
			}

			aesgcm, err := cipher.NewGCM(block)
			if err != nil {
				return nil, fmt.Errorf("failed to create GCM: %w", err)
			}

			plaintext, err := aesgcm.Open(nil, ec.Nonce, ec.Ciphertext, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt credential (wrong password?): %w", err)
			}

			var cred Credential
			if err := json.Unmarshal(plaintext, &cred); err != nil {
				return nil, fmt.Errorf("failed to unmarshal decrypted credential: %w", err)
			}

			return &cred, nil
		}
	}

	return nil, fmt.Errorf("credential not found")
}

// Upsert saves or updates a credential in file.
func (s *FileStore) Upsert(cred *Credential) error {
	if cred.CreatedAt.IsZero() {
		cred.CreatedAt = time.Now()
	}
	cred.UpdatedAt = time.Now()

	plaintext, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("failed to marshal credential: %w", err)
	}

	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	key := s.deriveKey(salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	file, err := s.loadFile()
	if err != nil {
		return err
	}

	found := false
	for i, ec := range file.Credentials {
		if ec.Service == cred.Service && ec.Account == cred.Account {
			file.Credentials[i] = encryptedCredential{
				Service:    cred.Service,
				Account:    cred.Account,
				Salt:       salt,
				Nonce:      nonce,
				Ciphertext: ciphertext,
			}
			found = true
			break
		}
	}

	if !found {
		file.Credentials = append(file.Credentials, encryptedCredential{
			Service:    cred.Service,
			Account:    cred.Account,
			Salt:       salt,
			Nonce:      nonce,
			Ciphertext: ciphertext,
		})
	}

	return s.saveFile(file)
}

// Delete removes a credential from file.
func (s *FileStore) Delete(service, account string) error {
	file, err := s.loadFile()
	if err != nil {
		return err
	}

	newCreds := []encryptedCredential{}
	for _, ec := range file.Credentials {
		if ec.Service == service && ec.Account == account {
			continue
		}
		newCreds = append(newCreds, ec)
	}

	file.Credentials = newCreds
	return s.saveFile(file)
}

// List returns accounts for a service.
func (s *FileStore) List(service string) ([]string, error) {
	file, err := s.loadFile()
	if err != nil {
		return nil, err
	}

	accounts := []string{}
	for _, ec := range file.Credentials {
		if ec.Service == service {
			accounts = append(accounts, ec.Account)
		}
	}
	return accounts, nil
}
