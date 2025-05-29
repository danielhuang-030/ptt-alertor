package user

import (
	"errors"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/models/subscription"
)

type User struct {
	Enable     bool      `json:"enable"`
	CreateTime time.Time `json:"createTime"`
	UpdateTime time.Time `json:"updateTime"`
	Profile    `json:"Profile"`
	Subscribes subscription.Subscriptions
	drive      Driver
}

// Profile is user's profile
type Profile struct {
	Account             string `json:"account"`
	Email               string `json:"email,omitempty"`
	Type                string `json:"type,omitempty"` // user, group, room
	Line                string `json:"line,omitempty"`
	LineAccessToken     string `json:"line_access_token,omitempty"`
	Messenger           string `json:"messenger,omitempty"`
	Telegram            string `json:"telegram,omitempty"`
	TelegramChat        int64  `json:"telegram_chat_id,omitempty"`
	DiscordChannelID    string `json:"discord_channel_id,omitempty"` // This will store the specific channel ID for notifications for this user profile
}

type Driver interface {
	List() (accounts []string)
	Exist(account string) bool
	Save(account string, user interface{}) error
	Update(account string, user interface{}) error
	Find(account string, user *User)
}

var ErrAccountEmpty = errors.New("account can not be empty")

func NewUser(drive Driver) *User {
	return &User{
		drive: drive,
	}
}

func (u User) All() (us []*User) {
	accounts := u.drive.List()
	for _, account := range accounts {
		user := u.Find(account)
		us = append(us, &user)
	}
	return us
}

func (u User) Save() error {

	if u.drive.Exist(u.Profile.Account) {
		return errors.New("user already exist")
	}

	if u.Profile.Account == "" {
		return ErrAccountEmpty
	}

	// If DiscordChannelID is not set, then at least one other contact method must be provided.
	if u.Profile.DiscordChannelID == "" {
		if u.Profile.Email == "" && u.Profile.Line == "" && u.Profile.Messenger == "" && u.Profile.Telegram == "" {
			return errors.New("one of Email, Line, Messenger, Telegram must be filled, or DiscordChannelID must be present")
		}
	}
	u.CreateTime = time.Now()
	u.UpdateTime = time.Now()

	return u.drive.Save(u.Profile.Account, u)
}

func (u User) Update() error {

	if !u.drive.Exist(u.Profile.Account) {
		return errors.New("user not exist")
	}

	if u.Profile.Account == "" {
		return ErrAccountEmpty
	}

	u.UpdateTime = time.Now()
	return u.drive.Update(u.Profile.Account, u)
}

func (u User) Find(account string) User {
	u.drive.Find(account, &u)
	return u
}
