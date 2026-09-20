package board

import (
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/models/article"
)

type stubDriver struct{}

func (stubDriver) GetArticles(string) article.Articles { return nil }
func (stubDriver) Save(string, article.Articles) error { return nil }
func (stubDriver) Delete(string) error                 { return nil }

type stubCacher struct{}

func (stubCacher) List() []string          { return nil }
func (stubCacher) Create(string) error     { return nil }
func (stubCacher) Exist(string) bool       { return false }
func (stubCacher) Remove(string) error     { return nil }

func TestFetchArticlesErrEmptyName(t *testing.T) {
	bd := NewBoard(stubDriver{}, stubCacher{})
	arts, err := bd.FetchArticlesErr()
	if err != nil {
		t.Fatalf("empty name should not error: %v", err)
	}
	if len(arts) != 0 {
		t.Fatalf("want empty articles, got %d", len(arts))
	}
}

func TestFetchArticlesWrapsErr(t *testing.T) {
	bd := NewBoard(stubDriver{}, stubCacher{})
	// Legacy FetchArticles must not panic and returns empty for empty name.
	if got := bd.FetchArticles(); len(got) != 0 {
		t.Fatalf("want empty, got %d", len(got))
	}
}

func TestWithNewArticlesErrSurfacesFetch(t *testing.T) {
	bd := NewBoard(stubDriver{}, stubCacher{})
	bd.Name = ""
	if err := bd.WithNewArticlesErr(); err != nil {
		t.Fatalf("empty name fetch should be nil err: %v", err)
	}
}
