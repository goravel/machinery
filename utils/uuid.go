package utils

import (
	"strings"

	"github.com/google/uuid"
)

func GetPureUUID() string {
	uid, _ := uuid.NewUUID()
	return strings.Replace(uid.String(), "-", "", -1)
}
