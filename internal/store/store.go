package store

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"boot.dev/linko/internal/linkoerr"
)

type ShortURL struct {
	ShortCode string `json:"short_code"`
	LongURL   string `json:"long_url"`
	Err       error  `json:"-"`
}

type storeErr string

func (e storeErr) Error() string {
	return string(e)
}

const (
	ErrNotFound = storeErr("not found")
)

type Store struct {
	dir    string
	logger *slog.Logger
}

func New(dir string, logger *slog.Logger) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{
		dir: dir,
	}, nil
}

func (s *Store) Create(_ context.Context, long string) (string, error) {
	const retries = 10
	const shortCodeLen = 6
	for range retries {
		short := rand.Text()[:shortCodeLen]
		path := filepath.Join(s.dir, short)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", err
		}
		defer f.Close()
		_, err = f.WriteString(long)
		if err != nil {
			return "", err
		}
		return short, nil
	}
	return "", errors.New("failed to generate unique short code")
}

const maxURLs = 10

func (s *Store) List(ctx context.Context) ([]ShortURL, error) {
	var errorcollector []error
	ch := make(chan ShortURL)
	go s.walk(ctx, ch)
	var urls []ShortURL
	for e := range ch {
		if e.Err != nil {
			errorcollector = append(errorcollector, e.Err)
			//return urls, e.Err
		}
		urls = append(urls, e)
		if len(urls) >= maxURLs {
			break
		}
	}
	return urls, errors.Join(errorcollector...)

}

func (s *Store) walk(ctx context.Context, ch chan<- ShortURL) {
	defer close(ch)
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			long, err := s.Lookup(ctx, e.Name())
			if err != nil {
				//ch <- ShortURL{Err: fmt.Errorf("read %s: %w", filepath.Join(s.dir, e.Name()), err)}
				ch <- ShortURL{Err: linkoerr.WithAttrs(err, "path", filepath.Join(s.dir, e.Name()))}
				continue
			}
			ch <- ShortURL{ShortCode: e.Name(), LongURL: long}
		}
	}
}

func (s *Store) Lookup(_ context.Context, short string) (string, error) {
	short = strings.ToUpper(short)
	shortcodeFilepath := filepath.Join(s.dir, short)
	data, err := os.ReadFile(shortcodeFilepath)
	if errors.Is(err, os.ErrNotExist) {

		return "", ErrNotFound
	}
	if err != nil {

		return "", err
	}

	return string(data), nil
}
