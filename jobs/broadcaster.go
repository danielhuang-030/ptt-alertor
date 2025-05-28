package jobs

import (
	"errors"

	"github.com/Ptt-Alertor/ptt-alertor/models"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

var platforms = map[string]bool{
	"email":     true,
	"line":      true,
	"messenger": true,
	"telegram":  true,
	"discord":   true,
}

type Broadcaster struct {
	Checker
	Msg string
}

func (bc Broadcaster) String() string {
	return bc.Msg
}

func (bc Broadcaster) Send(plfms []string) error {
	var platformBl = make(map[string]bool)
	for _, plfm := range plfms {
		if _, ok := platforms[plfm]; !ok {
			return errors.New("platform " + plfm + "is not in broadcast list")
		}
		platformBl[plfm] = true
	}

	for _, u := range models.User().All() {
		bc.subType = "broadcast"
		if platformBl["line"] {
			go bc.sendLine(u)
		}
		if platformBl["messenger"] {
			go bc.sendMessenger(u)
		}
		if platformBl["telegram"] {
			go bc.sendTelegram(u)
		}
		if platformBl["email"] {
			go bc.sendEmail(u)
		}
		if platformBl["discord"] {
			go bc.sendDiscord(u)
		}
	}
	return nil
}

func (bc Broadcaster) sendEmail(u *user.User) {
	bc.Profile.Email = u.Profile.Email
	ckCh <- bc
}

func (bc Broadcaster) sendLine(u *user.User) {
	bc.Profile.Line = u.Profile.Line
	bc.Profile.LineAccessToken = u.Profile.LineAccessToken
	ckCh <- bc
}

func (bc Broadcaster) sendMessenger(u *user.User) {
	bc.Profile.Messenger = u.Profile.Messenger
	ckCh <- bc
}

func (bc Broadcaster) sendTelegram(u *user.User) {
	bc.Profile.Telegram = u.Profile.Telegram
	bc.Profile.TelegramChat = u.Profile.TelegramChat
	ckCh <- bc
}

func (bc Broadcaster) sendDiscord(u *user.User) {
	if u.Profile.DiscordChannelID != "" { // 只在使用者設定了 DiscordChannelID 時才發送
		bc.Profile.DiscordChannelID = u.Profile.DiscordChannelID
		// 假設 Checker (ckCh 的接收端) 會處理 Profile 中的 DiscordChannelID
		// 並最終呼叫 discord.PushMessage(channelID, message, embed)
		// 目前我們只需要將帶有 DiscordChannelID 的 Broadcaster 物件發送到 ckCh
		bc.subType = "discord_broadcast" // 或一個更合適的 subType
		ckCh <- bc
	}
}
