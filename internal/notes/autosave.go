package notes

import (
	"errors"
	"os"
	"time"
)

var ErrConflict = errors.New("conflict: file modified on disk")

func CheckConflict(fullPath string, clientLastModifiedStr string) error {
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if clientLastModifiedStr != "" {
		clientTime, err := time.Parse(time.RFC3339, clientLastModifiedStr)
		if err == nil {
			if info.ModTime().After(clientTime.Add(time.Second)) {
				return ErrConflict
			}
		}
	}
	return nil
}
