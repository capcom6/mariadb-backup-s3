package main

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/capcom6/mariadb-backup-s3/internal/config"
)

func main() {
	cfg := config.Load()

	tempdir, err := os.MkdirTemp("", "mariadb")
	if err != nil {
		log.Fatalf("failed to create tempdir: %v", err)
	}
	defer os.RemoveAll(tempdir)

	log.Printf("tempdir: %s", tempdir)

	if err := backup(cfg.MariaDB, tempdir); err != nil {
		log.Fatalf("failed to backup: %v", err)
	}

	log.Printf("backup done: %s", tempdir)

	if err := prepare(cfg.MariaDB, tempdir); err != nil {
		log.Fatalf("failed to prepare: %v", err)
	}

	log.Printf("prepare done: %s", tempdir)

	compressed, err := os.CreateTemp("", "mariadb")
	if err != nil {
		log.Fatalf("failed to create compressed: %v", err)
	}
	defer os.Remove(compressed.Name())

	if err := compress(tempdir, compressed.Name()); err != nil {
		log.Fatalf("failed to compress: %v", err)
	}

	log.Printf("compressed done: %s", compressed.Name())

	if err := upload(cfg.Storage, compressed.Name()); err != nil {
		log.Fatalf("failed to upload: %v", err)
	}

	log.Printf("upload done: %s", compressed.Name())
}

func run(cmdline string) error {
	buf := bytes.Buffer{}

	cmd := exec.Command("bash", "-c", cmdline)

	cmd.Stdout = os.Stdout
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run: %w: %s", err, buf.String())
	}

	return nil
}

func backup(options config.MariaDB, dir string) error {
	cmdline := fmt.Sprintf(`mariabackup --backup --target-dir='%s' --user='%s' --password='%s'`, dir, options.User, options.Password)

	return run(cmdline)
}

func prepare(_ config.MariaDB, dir string) error {
	cmdline := fmt.Sprintf(`mariabackup --prepare --target-dir='%s'`, dir)

	return run(cmdline)
}

func compress(source, target string) error {
	cmdline := fmt.Sprintf(`tar -czf '%s' -C '%s' .`, target, source)

	return run(cmdline)
}

func upload(options config.Storage, source string) error {
	filename := time.Now().UTC().Format("2006-01-02-15-04-05") + ".tar.gz"

	parsedUrl, err := url.Parse(options.URL)
	if err != nil {
		return fmt.Errorf("failed to parse url %s: %w", options.URL, err)
	}

	if parsedUrl.Scheme != "s3" {
		return fmt.Errorf("unsupported scheme %s", parsedUrl.Scheme)
	}

	key := path.Join(parsedUrl.Path, filename)

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

	_, err = svc.PutObject(&s3.PutObjectInput{
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
