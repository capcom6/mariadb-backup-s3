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
	"math"

	"golang.org/x/crypto/hkdf"
)

// Service defines the interface for encryption operations.
type Service interface {
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

	keySize = 32 // 256 bits
	lenSize = 4  // 4 bytes
)

var (
	ErrInitializationFailed   = errors.New("initialization failed")
	ErrEncryptionFailed       = errors.New("encryption failed")
	ErrDecryptionFailed       = errors.New("decryption failed")
	ErrCiphertextTooLarge     = errors.New("ciphertext too large")
	ErrInvalidSaltNonceLength = errors.New("invalid salt/nonce length")
	ErrBadMagic               = errors.New("bad magic")
	ErrUnsupportedVersion     = errors.New("unsupported version")
)

// AES256GCMService implements AES-256-GCM encryption.
type AES256GCMService struct {
	masterKey []byte
}

// NewAES256GCMService creates a new AES-256-GCM encryption service.
func NewAES256GCMService(masterKey []byte) *AES256GCMService {
	return &AES256GCMService{
		masterKey: masterKey,
	}
}

func (s *AES256GCMService) prepare(salt []byte) (cipher.AEAD, error) {
	info := []byte("stream-encryption-v1")
	key, err := deriveKey(s.masterKey, salt, info)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInitializationFailed, err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create cipher block: %w", ErrInitializationFailed, err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create AEAD: %w", ErrInitializationFailed, err)
	}

	return aead, nil
}

// Encrypt encrypts a stream using AES-256-GCM with streaming support.
func (s *AES256GCMService) Encrypt(ctx context.Context, in io.Reader, out io.Writer) error {
	salt, err := generateSalt()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInitializationFailed, err)
	}

	noncePrefix, err := generateNoncePrefix()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInitializationFailed, err)
	}

	aead, err := s.prepare(salt)
	if err != nil {
		return err
	}

	if writeErr := s.writeHeader(out, salt, noncePrefix); writeErr != nil {
		return fmt.Errorf("%w: %w", ErrEncryptionFailed, writeErr)
	}

	buf := make([]byte, chunkSize)
	var chunkIdx uint64
	var nonce [nonceLen]byte
	copy(nonce[:noncePrefLen], noncePrefix)
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context done: %w", ctx.Err())
		default:
		}

		n, readErr := io.ReadFull(in, buf)
		if isEOF(readErr) {
			// final partial chunk (n > 0) or end with zero
			if n == 0 && readErr == io.EOF {
				break
			}
			// else proceed with n bytes and then break after
		} else if readErr != nil {
			return fmt.Errorf("failed to read plaintext: %w", readErr)
		}
		// build nonce: noncePrefix (4) || chunkIdx (8 BE)
		binary.BigEndian.PutUint64(nonce[noncePrefLen:], chunkIdx)

		// Use salt as authenticated additional data to bind chunks to stream
		ad := salt
		ciphertext := aead.Seal(nil, nonce[:], buf[:n], ad)

		// write frame
		if writeErr := writeFrame(out, ciphertext); writeErr != nil {
			return fmt.Errorf("%w: %w", ErrEncryptionFailed, writeErr)
		}

		chunkIdx++
		if isEOF(readErr) {
			break
		}
	}

	return nil
}

// decryptStream handles the streaming decryption logic.
func (s *AES256GCMService) decryptStream(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	aead cipher.AEAD,
	salt []byte,
	noncePref []byte,
) error {
	var chunkIdx uint64
	var lenBuf [lenSize]byte
	var ciphertext []byte
	var nonce [nonceLen]byte
	copy(nonce[:], noncePref)
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context done: %w", ctx.Err())
		default:
		}

		// read frame length
		if _, readErr := io.ReadFull(in, lenBuf[:]); readErr != nil {
			if readErr == io.EOF {
				return nil // normal end
			}
			return fmt.Errorf("%w: failed to read frame length: %w", ErrDecryptionFailed, readErr)
		}
		clen := binary.BigEndian.Uint32(lenBuf[:])
		if clen == 0 {
			return fmt.Errorf("%w: zero-length ciphertext frame", ErrDecryptionFailed)
		}

		if cap(ciphertext) < int(clen) {
			ciphertext = make([]byte, clen)
		} else {
			ciphertext = ciphertext[:clen]
		}

		if _, readErr := io.ReadFull(in, ciphertext); readErr != nil {
			return fmt.Errorf("%w: failed to read ciphertext: %w", ErrDecryptionFailed, readErr)
		}

		binary.BigEndian.PutUint64(nonce[noncePrefLen:], chunkIdx)

		// Use salt as authenticated additional data to bind chunks to stream
		ad := salt
		plaintext, aeadErr := aead.Open(nil, nonce[:], ciphertext, ad)
		if aeadErr != nil {
			return fmt.Errorf("%w: authentication failed: %w", ErrDecryptionFailed, aeadErr)
		}
		if _, wErr := out.Write(plaintext); wErr != nil {
			return fmt.Errorf("%w: failed to write plaintext: %w", ErrDecryptionFailed, wErr)
		}
		chunkIdx++
	}
}

// Decrypt decrypts a stream using AES-256-GCM with streaming support.
func (s *AES256GCMService) Decrypt(ctx context.Context, in io.Reader, out io.Writer) error {
	salt, noncePref, err := s.readHeader(in)
	if err != nil {
		return err
	}

	aead, err := s.prepare(salt)
	if err != nil {
		return err
	}

	return s.decryptStream(ctx, in, out, aead, salt, noncePref)
}

// writeHeader writes magic/version/salt/noncePrefix to out.
func (s *AES256GCMService) writeHeader(out io.Writer, salt []byte, noncePrefix []byte) error {
	if len(salt) != saltSize || len(noncePrefix) != noncePrefLen {
		return ErrInvalidSaltNonceLength
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
func (s *AES256GCMService) readHeader(r io.Reader) ([]byte, []byte, error) {
	h := make([]byte, len(magic))
	if _, err := io.ReadFull(r, h); err != nil {
		return nil, nil, fmt.Errorf("failed to read magic: %w", err)
	}
	if string(h) != magic {
		return nil, nil, fmt.Errorf("%w: %q", ErrBadMagic, h)
	}
	v := make([]byte, 1)
	if _, err := io.ReadFull(r, v); err != nil {
		return nil, nil, fmt.Errorf("failed to read version: %w", err)
	}
	if v[0] != version {
		return nil, nil, fmt.Errorf("%w: 0x%02x", ErrUnsupportedVersion, v)
	}
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(r, salt); err != nil {
		return nil, nil, fmt.Errorf("failed to read salt: %w", err)
	}
	noncePref := make([]byte, noncePrefLen)
	if _, err := io.ReadFull(r, noncePref); err != nil {
		return nil, nil, fmt.Errorf("failed to read nonce prefix: %w", err)
	}
	return salt, noncePref, nil
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
	key := make([]byte, keySize) // AES-256
	if _, err := io.ReadFull(hk, key); err != nil {
		return nil, fmt.Errorf("failed to derive a key: %w", err)
	}
	return key, nil
}

func isEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func writeFrame(out io.Writer, ciphertext []byte) error {
	// write frame length (uint32 BE)
	if uint64(len(ciphertext)) > math.MaxUint32 {
		return ErrCiphertextTooLarge
	}

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(ciphertext))) //nolint:gosec // validated early
	if _, writeErr := out.Write(lenBuf[:]); writeErr != nil {
		return fmt.Errorf("failed to write block size: %w", writeErr)
	}
	// write ciphertext
	if _, writeErr := out.Write(ciphertext); writeErr != nil {
		return fmt.Errorf("failed to write ciphertext: %w", writeErr)
	}

	return nil
}
