package encryption

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// EncryptionService defines the interface for encryption operations
type EncryptionService interface {
	// Encrypt encrypts a stream using AES-256-GCM with streaming support
	Encrypt(ctx context.Context, r io.Reader, w io.Writer) error
	// Decrypt decrypts a stream using AES-256-GCM with streaming support
	Decrypt(ctx context.Context, r io.Reader, w io.Writer) error
}

const (
	magic             = "SGCM"    // 4 bytes
	version      byte = 0x01      // 1 byte
	saltSize          = 16        // bytes for HKDF salt
	noncePrefLen      = 4         // 4 bytes random prefix
	nonceLen          = 12        // AES-GCM nonce length
	chunkSize         = 64 * 1024 // 64 KiB plaintext per chunk
)

var (
	ErrInitializationFailed = fmt.Errorf("initialization failed")
	ErrEncryptionFailed     = fmt.Errorf("encryption failed")
	ErrDecryptionFailed     = fmt.Errorf("decryption failed")
)

// AES256GCMService implements AES-256-GCM encryption
type AES256GCMService struct {
	masterKey []byte
}

// NewAES256GCMService creates a new AES-256-GCM encryption service
func NewAES256GCMService(masterKey []byte) *AES256GCMService {
	return &AES256GCMService{
		masterKey: masterKey,
	}
}

// Encrypt encrypts a stream using AES-256-GCM with streaming support
func (s *AES256GCMService) Encrypt(ctx context.Context, in io.Reader, out io.Writer) error {
	salt, err := generateSalt()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInitializationFailed, err)
	}

	noncePrefix, err := generateNoncePrefix()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInitializationFailed, err)
	}

	info := []byte("stream-encryption-v1")
	key, err := deriveKey(s.masterKey, salt, info)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInitializationFailed, err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("%w: failed to create cipher block: %w", ErrInitializationFailed, err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("%w: failed to create AEAD: %w", ErrInitializationFailed, err)
	}

	if err := s.writeHeader(out, salt, noncePrefix); err != nil {
		return fmt.Errorf("%w: %w", ErrEncryptionFailed, err)
	}

	buf := make([]byte, chunkSize)
	var chunkIdx uint64 = 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := io.ReadFull(in, buf)
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			// final partial chunk (n > 0) or end with zero
			if n == 0 && readErr == io.EOF {
				break
			}
			// else proceed with n bytes and then break after
		} else if readErr != nil {
			return readErr
		}
		// build nonce: noncePrefix (4) || chunkIdx (8 BE)
		nonce := make([]byte, nonceLen)
		copy(nonce, noncePrefix)
		binary.BigEndian.PutUint64(nonce[noncePrefLen:], chunkIdx)

		// Use salt as authenticated additional data to bind chunks to stream
		ad := salt
		ciphertext := aead.Seal(nil, nonce, buf[:n], ad)

		// write frame length (uint32 BE)
		if uint64(len(ciphertext)) > 0xFFFFFFFF {
			return errors.New("ciphertext too large")
		}
		var lenBuf [4]byte
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(ciphertext)))
		if _, err := out.Write(lenBuf[:]); err != nil {
			return err
		}
		// write ciphertext
		if _, err := out.Write(ciphertext); err != nil {
			return err
		}

		chunkIdx++
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
	}

	return nil
}

// Decrypt decrypts a stream using AES-256-GCM with streaming support
func (s *AES256GCMService) Decrypt(ctx context.Context, in io.Reader, out io.Writer) error {
	salt, noncePref, err := s.readHeader(in)
	if err != nil {
		return err
	}
	info := []byte("stream-encryption-v1")
	key, err := deriveKey(s.masterKey, salt, info)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	var chunkIdx uint64 = 0
	lenBuf := make([]byte, 4)
	for {
		// read frame length
		if _, err := io.ReadFull(in, lenBuf); err != nil {
			if err == io.EOF {
				return nil // normal end
			}
			return err
		}
		clen := binary.BigEndian.Uint32(lenBuf)
		if clen == 0 {
			return fmt.Errorf("%w: zero-length ciphertext frame", ErrDecryptionFailed)
		}
		ciphertext := make([]byte, clen)
		if _, err := io.ReadFull(in, ciphertext); err != nil {
			return err
		}

		nonce := make([]byte, nonceLen)
		copy(nonce, noncePref)
		binary.BigEndian.PutUint64(nonce[noncePrefLen:], chunkIdx)

		// Use salt as authenticated additional data to bind chunks to stream
		ad := salt
		plaintext, err := aead.Open(nil, nonce, ciphertext, ad)
		if err != nil {
			return err // auth failure (tampering or wrong key/nonce)
		}
		if _, err := out.Write(plaintext); err != nil {
			return err
		}
		chunkIdx++
	}
}

// writeHeader writes magic/version/salt/noncePrefix to out.
func (s *AES256GCMService) writeHeader(out io.Writer, salt []byte, noncePrefix []byte) error {
	if len(salt) != saltSize || len(noncePrefix) != noncePrefLen {
		return errors.New("invalid salt/noncePref length")
	}
	// header: magic(4) | version(1) | salt(16) | noncePref(4)
	if _, err := out.Write([]byte(magic)); err != nil {
		return fmt.Errorf("failed to write magic: %w", err)
	}
	if _, err := out.Write([]byte{version}); err != nil {
		return fmt.Errorf("failed to write version: %w", err)
	}
	if _, err := out.Write(salt); err != nil {
		return fmt.Errorf("failed to write salt: %w", err)
	}
	if _, err := out.Write(noncePrefix); err != nil {
		return fmt.Errorf("failed to write nonce prefix: %w", err)
	}
	return nil
}

// readHeader reads and validates header from r, returning salt and noncePrefix.
func (s *AES256GCMService) readHeader(r io.Reader) (salt, noncePref []byte, err error) {
	h := make([]byte, 4)
	if _, err = io.ReadFull(r, h); err != nil {
		return
	}
	if string(h) != magic {
		err = fmt.Errorf("bad magic: %q", h)
		return
	}
	v := make([]byte, 1)
	if _, err = io.ReadFull(r, v); err != nil {
		return
	}
	if v[0] != version {
		err = fmt.Errorf("unsupported version: %d", v[0])
		return
	}
	salt = make([]byte, saltSize)
	if _, err = io.ReadFull(r, salt); err != nil {
		return
	}
	noncePref = make([]byte, noncePrefLen)
	if _, err = io.ReadFull(r, noncePref); err != nil {
		return
	}
	return
}

func generateSalt() ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	return salt, nil
}

func generateNoncePrefix() ([]byte, error) {
	noncePrefix := make([]byte, noncePrefLen)
	if _, err := rand.Read(noncePrefix); err != nil {
		return nil, fmt.Errorf("failed to generate nonce prefix: %w", err)
	}

	return noncePrefix, nil
}

// deriveKey derives a 32-byte AES key using HKDF-SHA256 from masterKey + salt.
func deriveKey(masterKey, salt []byte, info []byte) ([]byte, error) {
	h := sha256.New
	hk := hkdf.New(h, masterKey, salt, info)
	key := make([]byte, 32) // AES-256
	if _, err := io.ReadFull(hk, key); err != nil {
		return nil, err
	}
	return key, nil
}
