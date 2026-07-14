package tui

import (
	"context"
	"errors"
	"testing"

	"auris/pkg/locale"
	"auris/pkg/portfolio"
)

func TestModelsLoadedMsg_Err_ResetsPendingPortfolioCreate(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	a := NewApp(AppOptions{})
	a.styles = NewStyles(ThemeDark)
	a.pendingPortfolioCreate = true
	a.screen = ScreenPortfolioMenu
	a.current = newPortfolioMenuModel(a.styles, nil)

	updated, _ := a.Update(modelsLoadedMsg{err: context.DeadlineExceeded})
	a = updated.(*AppModel)

	if a.pendingPortfolioCreate {
		t.Fatal("expected pendingPortfolioCreate to be reset after a load error")
	}
	m, ok := a.current.(*portfolioMenuModel)
	if !ok {
		t.Fatalf("expected *portfolioMenuModel, got %T", a.current)
	}
	if m.err == "" {
		t.Fatal("expected an error message to be surfaced on the portfolio menu screen")
	}
}

func TestModelsLoadedMsg_Err_ResetsPendingPortfolioEdit(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	a := NewApp(AppOptions{})
	a.styles = NewStyles(ThemeDark)
	a.pendingPortfolioEdit = true
	a.activePortfolio = portfolio.NewPortfolio("Test")
	a.screen = ScreenPortfolioView
	a.current = newPortfolioViewModel(a.activePortfolio, nil, a.styles, 0)

	updated, _ := a.Update(modelsLoadedMsg{err: errors.New("boom")})
	a = updated.(*AppModel)

	if a.pendingPortfolioEdit {
		t.Fatal("expected pendingPortfolioEdit to be reset after a load error")
	}
	m, ok := a.current.(*portfolioViewModel)
	if !ok {
		t.Fatalf("expected *portfolioViewModel, got %T", a.current)
	}
	if m.infoMsg == "" {
		t.Fatal("expected an error message to be surfaced on the portfolio view screen")
	}
}

func TestModelsLoadedMsg_Err_SurfacedOnMenu(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	a := NewApp(AppOptions{})
	a.styles = NewStyles(ThemeDark)
	a.screen = ScreenMenu
	a.current = newMenuModel(a.styles, false, false)

	updated, _ := a.Update(modelsLoadedMsg{err: errors.New("boom")})
	a = updated.(*AppModel)

	m, ok := a.current.(*MenuModel)
	if !ok {
		t.Fatalf("expected *MenuModel, got %T", a.current)
	}
	if m.err == "" {
		t.Fatal("expected an error message to be surfaced on the menu screen")
	}
}

func TestModelsLoadedMsg_Success_DoesNotSetError(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	a := NewApp(AppOptions{})
	a.styles = NewStyles(ThemeDark)
	a.pendingPortfolioCreate = true
	a.screen = ScreenPortfolioMenu
	a.current = newPortfolioMenuModel(a.styles, nil)

	updated, _ := a.Update(modelsLoadedMsg{})
	a = updated.(*AppModel)

	if a.pendingPortfolioCreate {
		t.Fatal("expected pendingPortfolioCreate to be cleared on success")
	}
	if a.screen != ScreenPortfolioCreate {
		t.Fatalf("expected to transition to ScreenPortfolioCreate, got %v", a.screen)
	}
}
