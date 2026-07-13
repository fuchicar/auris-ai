package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"auris/pkg/locale"
	"auris/pkg/news"
)

func TestNewsFeedsModel_BuildCatalog(t *testing.T) {
	// Active = every default feed except the first one, plus one custom feed.
	active := make([]news.FeedConfig, 0, len(news.DefaultFeeds))
	for _, f := range news.DefaultFeeds[1:] {
		active = append(active, f)
	}
	custom := news.FeedConfig{Name: "My Feed", URL: "https://example.com/rss.xml", Language: "en"}
	active = append(active, custom)

	entries, selected := buildNewsFeedCatalog(active)

	if len(entries) != len(news.DefaultFeeds)+1 {
		t.Fatalf("len(entries) = %d, want %d", len(entries), len(news.DefaultFeeds)+1)
	}
	if !entries[0].IsDefault || entries[0].Feed.URL != news.DefaultFeeds[0].URL {
		t.Errorf("entries[0] = %+v, want first default feed", entries[0])
	}
	last := entries[len(entries)-1]
	if last.IsDefault || last.Feed.URL != custom.URL {
		t.Errorf("last entry = %+v, want custom feed %+v", last, custom)
	}
	if selected[news.DefaultFeeds[0].URL] {
		t.Error("first default feed should not be selected (it was excluded from active)")
	}
	if !selected[custom.URL] {
		t.Error("custom feed should be selected")
	}
	if !selected[news.DefaultFeeds[1].URL] {
		t.Error("second default feed should be selected")
	}
}

func TestNewsFeedsModel_ToggleSelection(t *testing.T) {
	m := newNewsFeedsModel(NewStyles(ThemeDark), news.DefaultFeeds, true)
	url := m.entries[0].Feed.URL

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(*NewsFeedsModel)
	if m.selected[url] {
		t.Error("Space on cursor 0 should have deselected it")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(*NewsFeedsModel)
	if !m.selected[url] {
		t.Error("Space again should reselect it")
	}
}

func TestNewsFeedsModel_DeleteCustomOnly(t *testing.T) {
	custom := news.FeedConfig{Name: "My Feed", URL: "https://example.com/rss.xml", Language: "en"}
	active := append([]news.FeedConfig{}, news.DefaultFeeds...)
	active = append(active, custom)
	m := newNewsFeedsModel(NewStyles(ThemeDark), active, true)

	// Cursor is at 0, a default entry — 'd' should be a no-op.
	before := len(m.entries)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(*NewsFeedsModel)
	if len(m.entries) != before {
		t.Fatalf("'d' on a default entry should be a no-op, len(entries) went from %d to %d", before, len(m.entries))
	}

	// Move cursor to the custom entry (last one) and delete it.
	m.cursor = len(m.entries) - 1
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(*NewsFeedsModel)
	if len(m.entries) != before-1 {
		t.Fatalf("'d' on the custom entry should remove it, len(entries) = %d, want %d", len(m.entries), before-1)
	}
	if m.selected[custom.URL] {
		t.Error("deleted custom feed should be removed from selected too")
	}
}

func TestNewsFeedsModel_AddFeed_Flow(t *testing.T) {
	m := newNewsFeedsModel(NewStyles(ThemeDark), nil, true)
	before := len(m.entries)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(*NewsFeedsModel)
	if m.mode != nfModeAdd {
		t.Fatal("'a' should switch to add mode")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("My Feed")})
	m = updated.(*NewsFeedsModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*NewsFeedsModel)
	if m.addStep != nfAddStepURL {
		t.Fatalf("after entering a name, addStep = %v, want nfAddStepURL", m.addStep)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://example.com/rss.xml")})
	m = updated.(*NewsFeedsModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*NewsFeedsModel)
	if m.addStep != nfAddStepLanguage {
		t.Fatalf("after entering a URL, addStep = %v, want nfAddStepLanguage", m.addStep)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("en")})
	m = updated.(*NewsFeedsModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*NewsFeedsModel)

	if m.mode != nfModeList {
		t.Fatal("after entering language, should return to list mode")
	}
	if len(m.entries) != before+1 {
		t.Fatalf("len(entries) = %d, want %d", len(m.entries), before+1)
	}
	newEntry := m.entries[len(m.entries)-1]
	if newEntry.IsDefault {
		t.Error("newly added feed should not be marked as default")
	}
	if newEntry.Feed.Name != "My Feed" || newEntry.Feed.URL != "https://example.com/rss.xml" || newEntry.Feed.Language != "en" {
		t.Errorf("new feed = %+v, unexpected fields", newEntry.Feed)
	}
	if !m.selected[newEntry.Feed.URL] {
		t.Error("newly added feed should be selected by default")
	}
}

func TestNewsFeedsModel_AddFeed_ValidationErrors(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}

	m := newNewsFeedsModel(NewStyles(ThemeDark), nil, true)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(*NewsFeedsModel)

	// Empty name should not advance.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*NewsFeedsModel)
	if m.addStep != nfAddStepName {
		t.Fatalf("empty name should not advance the step, got %v", m.addStep)
	}
	if m.addErr == "" {
		t.Error("empty name should set addErr")
	}

	// Fill in a valid name, then try an invalid URL.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("My Feed")})
	m = updated.(*NewsFeedsModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*NewsFeedsModel)
	if m.addStep != nfAddStepURL {
		t.Fatalf("valid name should advance to nfAddStepURL, got %v", m.addStep)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("not-a-url")})
	m = updated.(*NewsFeedsModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*NewsFeedsModel)
	if m.addStep != nfAddStepURL {
		t.Fatalf("invalid URL should not advance the step, got %v", m.addStep)
	}
	if m.addErr == "" {
		t.Error("invalid URL should set addErr")
	}
}

func TestNewsFeedsModel_EnterEmitsFilteredOrderedResult(t *testing.T) {
	m := newNewsFeedsModel(NewStyles(ThemeDark), news.DefaultFeeds, true)

	// Uncheck the first default.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(*NewsFeedsModel)

	// Add and keep a custom feed.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(*NewsFeedsModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("My Feed")})
	m = updated.(*NewsFeedsModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*NewsFeedsModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://example.com/rss.xml")})
	m = updated.(*NewsFeedsModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*NewsFeedsModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("en")})
	m = updated.(*NewsFeedsModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*NewsFeedsModel)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command emitting ScreenDoneMsg, got nil")
	}
	msg, ok := cmd().(ScreenDoneMsg)
	if !ok {
		t.Fatalf("expected ScreenDoneMsg, got %T", msg)
	}
	if msg.From != ScreenNewsFeeds {
		t.Errorf("From = %v, want ScreenNewsFeeds", msg.From)
	}
	res, ok := msg.Result.(NewsFeedsResult)
	if !ok {
		t.Fatalf("expected NewsFeedsResult, got %T", msg.Result)
	}
	wantLen := len(news.DefaultFeeds) - 1 + 1
	if len(res.Feeds) != wantLen {
		t.Fatalf("len(Feeds) = %d, want %d", len(res.Feeds), wantLen)
	}
	if res.Feeds[0].URL != news.DefaultFeeds[1].URL {
		t.Errorf("Feeds[0] = %+v, want second default feed (first was unchecked)", res.Feeds[0])
	}
	last := res.Feeds[len(res.Feeds)-1]
	if last.URL != "https://example.com/rss.xml" {
		t.Errorf("last feed = %+v, want the custom feed", last)
	}
}

func TestNewsFeedsModel_EscCancelsWithNilResult(t *testing.T) {
	m := newNewsFeedsModel(NewStyles(ThemeDark), news.DefaultFeeds, true)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command emitting ScreenDoneMsg, got nil")
	}
	msg, ok := cmd().(ScreenDoneMsg)
	if !ok {
		t.Fatalf("expected ScreenDoneMsg, got %T", msg)
	}
	if msg.Result != nil {
		t.Errorf("Result = %v, want nil (cancelled)", msg.Result)
	}
}

func TestNewsFeedsModel_EscNoOpWhenCannotGoBack(t *testing.T) {
	m := newNewsFeedsModel(NewStyles(ThemeDark), news.DefaultFeeds, false)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		if _, ok := cmd().(ScreenDoneMsg); ok {
			t.Fatal("Esc should not cancel when canGoBack is false")
		}
	}
}

func TestNewsFeedsModel_ScrollFollowsCursor(t *testing.T) {
	m := newNewsFeedsModel(NewStyles(ThemeDark), news.DefaultFeeds, true)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 15})
	m = updated.(*NewsFeedsModel)

	maxVis := m.maxVisible()
	if maxVis >= len(m.entries) {
		t.Fatalf("test assumes the catalog (%d entries) overflows maxVisible (%d)", len(m.entries), maxVis)
	}

	// Move the cursor past the initial visible window and confirm scrollOff follows it.
	for i := 0; i < maxVis+2; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(*NewsFeedsModel)
	}
	if m.cursor < m.scrollOff || m.cursor >= m.scrollOff+maxVis {
		t.Fatalf("cursor %d is outside the visible window [%d, %d)", m.cursor, m.scrollOff, m.scrollOff+maxVis)
	}
	if m.scrollOff == 0 {
		t.Error("scrollOff should have advanced past 0 once the cursor left the initial window")
	}
}

func TestNewsFeedsModel_View(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	m := newNewsFeedsModel(NewStyles(ThemeDark), news.DefaultFeeds, true)
	if got := m.View(); got == "" {
		t.Fatal("View() returned empty string")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(*NewsFeedsModel)
	if got := m.View(); got == "" {
		t.Fatal("View() in add mode returned empty string")
	}
}
