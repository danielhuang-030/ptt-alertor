package models

import (
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

var User = func() *user.User {
	return user.NewUser(new(user.Redis))
}
var Article = func() *article.Article {
	return article.NewArticle(new(article.DynamoDB))
}
var Board = func() *board.Board {
	// 舊的硬式編碼邏輯:
	// return board.NewBoard(new(board.DynamoDB), new(board.Redis))

	// 新的邏輯，使用 board_setup.go 中初始化的 Driver 和 Cacher:
	return board.NewBoard(board.GetDefaultBoardDriver(), board.GetDefaultBoardCacher())
}
