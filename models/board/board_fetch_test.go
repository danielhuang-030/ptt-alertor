package board

import (
	"errors"
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/rss"
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

func TestFetchArticlesErrRateLimitNoHTMLFallback(t *testing.T) {
	origRSS, origWeb := rssBuildArticles, webFetchArticles
	t.Cleanup(func() {
		rssBuildArticles = origRSS
		webFetchArticles = origWeb
	})

	htmlCalled := false
	rssBuildArticles = func(string) (article.Articles, error) {
		return nil, rss.ErrTooManyRequests
	}
	webFetchArticles = func(string, int) (article.Articles, error) {
		htmlCalled = true
		return article.Articles{{Title: "should-not-use"}}, nil
	}

	bd := NewBoard(stubDriver{}, stubCacher{})
	bd.Name = "Gossiping"
	arts, err := bd.FetchArticlesErr()
	if err != rss.ErrTooManyRequests {
		t.Fatalf("want ErrTooManyRequests, got %v", err)
	}
	if htmlCalled {
		t.Fatal("HTML fallback must not run on ErrTooManyRequests")
	}
	if len(arts) != 0 {
		t.Fatalf("want nil/empty articles, got %d", len(arts))
	}
}

func TestFetchArticlesErrBothFailReturnsHTMLErr(t *testing.T) {
	origRSS, origWeb := rssBuildArticles, webFetchArticles
	t.Cleanup(func() {
		rssBuildArticles = origRSS
		webFetchArticles = origWeb
	})

	rssErr := errors.New("rss boom")
	htmlErr := errors.New("html boom")
	rssBuildArticles = func(string) (article.Articles, error) {
		return nil, rssErr
	}
	webFetchArticles = func(string, int) (article.Articles, error) {
		return nil, htmlErr
	}

	bd := NewBoard(stubDriver{}, stubCacher{})
	bd.Name = "Gossiping"
	_, err := bd.FetchArticlesErr()
	if err != htmlErr {
		t.Fatalf("want htmlErr, got %v", err)
	}
}

func TestFetchArticlesErrHTMLFallbackSuccess(t *testing.T) {
	origRSS, origWeb := rssBuildArticles, webFetchArticles
	t.Cleanup(func() {
		rssBuildArticles = origRSS
		webFetchArticles = origWeb
	})

	want := article.Articles{{Title: "from-html", Board: "Gossiping"}}
	rssBuildArticles = func(string) (article.Articles, error) {
		return nil, errors.New("rss boom")
	}
	webFetchArticles = func(string, int) (article.Articles, error) {
		return want, nil
	}

	bd := NewBoard(stubDriver{}, stubCacher{})
	bd.Name = "Gossiping"
	arts, err := bd.FetchArticlesErr()
	if err != nil {
		t.Fatalf("want nil err, got %v", err)
	}
	if len(arts) != 1 || arts[0].Title != "from-html" {
		t.Fatalf("want HTML articles, got %+v", arts)
	}
}
