package app

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/credentials"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

var (
	ErrInvalidCredentials   = errors.New("invalid username or password")
	ErrDirectoryUnavailable = errors.New("directory is unavailable")
	ErrManagedByDirectory   = errors.New("the account is managed by the directory")
	ErrWrongPassword        = errors.New("the current password is wrong")
	ErrSamePassword         = errors.New("the new password equals the current one")
	ErrAvatarInvalid        = errors.New("the file is not a supported image")
	ErrAvatarTooLarge       = errors.New("the image is too large")
	ErrTooManyAttempts      = errors.New("too many failed attempts")
	ErrNotFound             = errors.New("not found")
	ErrUnauthenticated      = errors.New("unauthenticated")
	ErrPasswordExpired      = errors.New("the password has expired")
	ErrNoLocalAdmin         = errors.New("no active local administrator would remain")

	invalidCSRF = errors.New("csrf")
	forbidden   = errors.New("forbidden")
)

type InputError struct {
	Code string
	Err  error
}

func (e *InputError) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return e.Code + ": " + e.Err.Error()
}

func invalid(code string, err error) error { return &InputError{Code: code, Err: err} }

var statuses = []struct {
	err    error
	status int
	code   string
}{
	{ErrInvalidCredentials, http.StatusUnauthorized, "invalid_credentials"},
	{ErrUnauthenticated, http.StatusUnauthorized, "unauthenticated"},
	{ErrDirectoryUnavailable, http.StatusServiceUnavailable, "directory_unavailable"},
	{credentials.ErrUnavailable, http.StatusServiceUnavailable, "secrets_unavailable"},
	{ErrManagedByDirectory, http.StatusConflict, "managed_by_directory"},
	{ErrWrongPassword, http.StatusForbidden, "wrong_password"},
	{ErrSamePassword, http.StatusBadRequest, "same_password"},
	{ErrAvatarInvalid, http.StatusBadRequest, "avatar_invalid"},
	{ErrAvatarTooLarge, http.StatusRequestEntityTooLarge, "avatar_too_large"},
	{ErrTooManyAttempts, http.StatusTooManyRequests, "too_many_attempts"},
	{ErrNotFound, http.StatusNotFound, "not_found"},
	{invalidCSRF, http.StatusForbidden, "csrf"},
	{forbidden, http.StatusForbidden, "forbidden"},
	{ErrPasswordExpired, http.StatusForbidden, "password_expired"},
	{ErrNoLocalAdmin, http.StatusConflict, "no_local_admin"},
}

func writeError(w http.ResponseWriter, err error) {
	var pe *model.PolicyError
	if errors.As(err, &pe) {
		httpx.JSON(w, http.StatusBadRequest, httpx.Problem{Code: "weak_password", Detail: err.Error(), Violations: pe.Violations})
		return
	}
	var ie *InputError
	if errors.As(err, &ie) {
		httpx.Error(w, http.StatusBadRequest, ie.Code, ie.Err)
		return
	}
	for _, s := range statuses {
		if errors.Is(err, s.err) {
			httpx.Error(w, s.status, s.code, nil)
			return
		}
	}
	slog.Error("request failed", "err", err)
	httpx.Error(w, http.StatusInternalServerError, "internal", nil)
}
