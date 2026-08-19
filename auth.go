package main

import (
	"context"

	"net/http"

	"log/slog"

	pkgerr "github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const UserContextKey contextKey = "user"

var allowedUsers = map[string]string{
	"frodo":   "$2a$10$B6O/n6teuCzpuh66jrUAdeaJ3WvXcxRkzpN0x7H.di9G9e/NGb9Me",
	"samwise": "$2a$10$EWZpvYhUJtJcEMmm/IBOsOGIcpxUnGIVMRiDlN/nxl1RRwWGkJtty",
	// frodo: "ofTheNineFingers"
	// samwise: "theStrong"
	"saruman": "invalidFormat",
}

func (s *server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		stored, exists := allowedUsers[user]
		if !exists {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		ok, err := s.validatePassword(password, stored)
		if err != nil {
			s.logger.Error("error validating password",
				slog.String("user", user),
				"error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), UserContextKey, user))
		next.ServeHTTP(w, r)
	})
}

func (s *server) validatePassword(password, stored string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(password))
	if err == bcrypt.ErrMismatchedHashAndPassword {
		return false, nil
	}
	if err != nil {
		//s.logger.Error("error validating password", slog.String("error", fmt.Sprintf("%v", err)))
		return false, pkgerr.WithStack(err)
	}
	return true, nil
}
