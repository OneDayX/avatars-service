package domain

import "errors"

var (
	ErrNotFound      = errors.New("avatar not found")
	ErrForbidden     = errors.New("forbidden")
	ErrInvalidFile   = errors.New("invalid file format")
	ErrFileTooLarge  = errors.New("file too large")
	ErrNotReady      = errors.New("avatar is still processing")
	ErrInvalidUserID = errors.New("invalid user id")
	ErrInvalidSize   = errors.New("invalid avatar size")
)
