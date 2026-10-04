package app

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

type userView struct {
	model.User
	CSRF          string `json:"csrf,omitempty"`
	Gravatar      string `json:"gravatar,omitempty"`
	HasAvatar     bool   `json:"has_avatar"`
	AvatarVersion string `json:"avatar_version,omitempty"`
}

func view(u model.User, csrf string) userView {
	v := userView{User: u, CSRF: csrf, HasAvatar: len(u.Avatar) > 0}
	if e := strings.ToLower(strings.TrimSpace(u.Email)); e != "" {
		h := sha256.Sum256([]byte(e))
		v.Gravatar = hex.EncodeToString(h[:])
	}
	if u.AvatarAt != nil && v.HasAvatar {
		v.AvatarVersion = strconv.FormatInt(u.AvatarAt.UnixNano(), 10)
	}
	return v
}
