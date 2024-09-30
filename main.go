package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

var ErrInterrupted = fmt.Errorf("interrupted")

var cores = runtime.NumCPU()

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := doWork(ctx); err != nil {
		log.Fatal(err)
	}
}

func doWork(ctx context.Context) error {
	cfg := config.Load()

	tempdir, err := os.MkdirTemp("", "mariadb")
	if err != nil {
		return fmt.Errorf("failed to create tempdir: %w", err)
	}
	defer os.RemoveAll(tempdir)
	log.Printf("tempdir: %s", tempdir)

	if err := backup(ctx, cfg.MariaDB, tempdir); err != nil {
		return fmt.Errorf("failed to backup: %w", err)
	}
	log.Printf("backup done: %s", tempdir)

	if err := prepare(ctx, cfg.MariaDB, tempdir); err != nil {
		return fmt.Errorf("failed to prepare: %w", err)
	}
	log.Printf("prepare done: %s", tempdir)

	compressed, err := os.CreateTemp("", "mariadb")
	if err != nil {
		return fmt.Errorf("failed to create compressed: %w", err)
	}
	defer os.Remove(compressed.Name())

	if err := compress(ctx, tempdir, compressed.Name()); err != nil {
		return fmt.Errorf("failed to compress: %w", err)
	}
	log.Printf("compressed done: %s", compressed.Name())

	if err := upload(ctx, cfg.Backup, cfg.Storage, compressed.Name()); err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}
	log.Printf("upload done: %s", compressed.Name())

	return nil
}

func run(_ context.Context, cmdline string) error {
	buf := bytes.Buffer{}

	cmd := exec.Command("bash", "-c", cmdline)

	cmd.Stdout = os.Stdout
	cmd.Stderr = &buf

	return cmd.Run()
}

func backup(ctx context.Context, options config.MariaDB, dir string) error {
	cmdline := fmt.Sprintf(`mariabackup --backup --parallel=%d --target-dir='%s' --user='%s' --password='%s'`, cores, dir, options.User, options.Password)

	return run(ctx, cmdline)
}

func prepare(ctx context.Context, _ config.MariaDB, dir string) error {
	cmdline := fmt.Sprintf(`mariabackup --prepare --target-dir='%s'`, dir)

	return run(ctx, cmdline)
}

func compress(ctx context.Context, source, target string) error {
	// cmdline := fmt.Sprintf(`tar -czf '%s' -C '%s' .`, target, source)
	cmdline := fmt.Sprintf(`tar -cf - '%s' | pigz > '%s'`, source, target)

	return run(ctx, cmdline)
}

func upload(ctx context.Context, backup config.Backup, storage config.Storage, source string) error {
	filename := time.Now().UTC().Format("2006-01-02-15-04-05") + ".tar.gz"

	parsedUrl, err := url.Parse(storage.URL)
	if err != nil {
		return fmt.Errorf("failed to parse url %s: %w", storage.URL, err)
	}

	if parsedUrl.Scheme != "s3" {
		return fmt.Errorf("unsupported scheme %s", parsedUrl.Scheme)
	}

	prefix := strings.Trim(parsedUrl.Path, "/")
	key := strings.TrimPrefix(path.Join(parsedUrl.Path, filename), "/")

	h, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", source, err)
	}
	defer h.Close()

	var endpoint *string
	var forcePathStyle *bool

	if val := parsedUrl.Query().Get("endpoint"); val != "" {
		endpoint = aws.String(val)
	}
	if val := parsedUrl.Query().Get("force_path_style"); val != "" {
		forcePathStyle = aws.Bool(val == "true")
	}

	sess, err := session.NewSession(&aws.Config{
		Endpoint:         endpoint,
		S3ForcePathStyle: forcePathStyle,
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	svc := s3.New(sess)

	if err := cleanup(ctx, backup, svc, parsedUrl.Host, prefix); err != nil {
		log.Printf("failed to cleanup: %s", err)
	}

	_, err = svc.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket:      &parsedUrl.Host,
		Key:         &key,
		ContentType: aws.String("application/x-gzip"),
		Body:        h,
	})
	if err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}

	return nil
}

func cleanup(ctx context.Context, backup config.Backup, svc *s3.S3, bucket, prefix string) error {
	if backup.Limits.MaxCount == 0 {
		return nil
	}

	keys := make([]*s3.ObjectIdentifier, 0, backup.Limits.MaxCount+1)

	err := svc.ListObjectsV2PagesWithContext(ctx, &s3.ListObjectsV2Input{
		Bucket: &bucket,
		Prefix: aws.String(prefix),
	}, func(p *s3.ListObjectsV2Output, last bool) bool {
		for _, obj := range p.Contents {
			keys = append(keys, &s3.ObjectIdentifier{Key: obj.Key})
		}
		return true
	})
	if err != nil {
		return fmt.Errorf("failed to cleanup: %w", err)
	}

	if len(keys) <= backup.Limits.MaxCount {
		return nil
	}

	log.Printf("found %d keys, %d will be deleted", len(keys), len(keys)-backup.Limits.MaxCount)

	_, err = svc.DeleteObjectsWithContext(ctx, &s3.DeleteObjectsInput{
		Bucket: &bucket,
		Delete: &s3.Delete{
			Objects: keys[:len(keys)-backup.Limits.MaxCount],
		},
	})
	if err != nil {
		return fmt.Errorf("failed to cleanup: %w", err)
	}

	return nil
}

func isInterrupted(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
