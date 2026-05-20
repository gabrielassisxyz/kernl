package ids

import "github.com/google/uuid"

func New() string {
	u, _ := uuid.NewV7()
	return u.String()
}
