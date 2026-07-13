package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
	"auris/pkg/news"
)

// newsFeedEntry is one row in the merged catalog shown by [NewsFeedsModel]:
// every entry from news.DefaultFeeds plus any custom feeds already present
// in the user's config that aren't part of the default set.
type newsFeedEntry struct {
	Feed      news.FeedConfig
	IsDefault bool
}

// newsFeedsMode selects which sub-view [NewsFeedsModel] renders.
type newsFeedsMode int

const (
	nfModeList newsFeedsMode = iota
	nfModeAdd
)

// newsFeedAddStep tracks the current field of the "add a custom feed" form.
type newsFeedAddStep int

const (
	nfAddStepName newsFeedAddStep = iota
	nfAddStepURL
	nfAddStepLanguage
)

// NewsFeedsModel lets the user choose which of news.DefaultFeeds (plus any
// custom feeds already in cfg.NewsFeeds) are active, and add/remove custom
// feeds (FEAT-14). Reached from the Configuration menu via /news.
type NewsFeedsModel struct {
	mode      newsFeedsMode
	entries   []newsFeedEntry // defaults (DefaultFeeds order) + customs (add order)
	selected  map[string]bool // keyed by Feed.URL
	cursor    int
	scrollOff int // first visible entry index
	height    int // terminal height (updated by WindowSizeMsg)
	canGoBack bool
	styles    *Styles

	addStep   newsFeedAddStep
	nameInput textinput.Model
	urlInput  textinput.Model
	langInput textinput.Model
	addErr    string
}

// buildNewsFeedCatalog merges news.DefaultFeeds with any custom entries
// already present in active (i.e. cfg.NewsFeeds) that aren't part of the
// default set, and returns the initial "currently active" selection.
func buildNewsFeedCatalog(active []news.FeedConfig) ([]newsFeedEntry, map[string]bool) {
	defaultURLs := make(map[string]bool, len(news.DefaultFeeds))
	entries := make([]newsFeedEntry, 0, len(news.DefaultFeeds)+len(active))
	for _, f := range news.DefaultFeeds {
		defaultURLs[f.URL] = true
		entries = append(entries, newsFeedEntry{Feed: f, IsDefault: true})
	}

	seenCustom := make(map[string]bool)
	for _, f := range active {
		if defaultURLs[f.URL] || seenCustom[f.URL] {
			continue
		}
		seenCustom[f.URL] = true
		entries = append(entries, newsFeedEntry{Feed: f, IsDefault: false})
	}

	selected := make(map[string]bool, len(active))
	for _, f := range active {
		selected[f.URL] = true
	}

	return entries, selected
}

// newNewsFeedsModel constructs a [NewsFeedsModel], preselecting whichever
// feeds are in active (typically AppModel.cfg.NewsFeeds).
func newNewsFeedsModel(s *Styles, active []news.FeedConfig, canGoBack bool) *NewsFeedsModel {
	entries, selected := buildNewsFeedCatalog(active)

	nameInput := textinput.New()
	nameInput.CharLimit = 80
	urlInput := textinput.New()
	urlInput.CharLimit = 300
	langInput := textinput.New()
	langInput.CharLimit = 10

	return &NewsFeedsModel{
		entries:   entries,
		selected:  selected,
		canGoBack: canGoBack,
		styles:    s,
		nameInput: nameInput,
		urlInput:  urlInput,
		langInput: langInput,
	}
}

// Init implements [tea.Model].
func (m *NewsFeedsModel) Init() tea.Cmd { return nil }

// maxVisible returns the number of catalog rows that fit in the terminal.
func (m *NewsFeedsModel) maxVisible() int {
	if m.height == 0 {
		return 20
	}
	n := m.height - 5 // overhead: title + hint + scroll indicators + margin
	if n < 3 {
		return 3
	}
	return n
}

// Update implements [tea.Model].
func (m *NewsFeedsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = ws.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.mode == nfModeAdd {
		return m.handleAddKey(key)
	}
	return m.handleListKey(key)
}

// handleListKey handles keystrokes while browsing the feed catalog.
func (m *NewsFeedsModel) handleListKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
		}
	case tea.KeyDown:
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
		}
	case tea.KeySpace:
		if len(m.entries) > 0 {
			url := m.entries[m.cursor].Feed.URL
			m.selected[url] = !m.selected[url]
		}
	case tea.KeyEsc:
		if m.canGoBack {
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenNewsFeeds, Result: nil}
			}
		}
	case tea.KeyEnter:
		var feeds []news.FeedConfig
		for _, e := range m.entries {
			if m.selected[e.Feed.URL] {
				feeds = append(feeds, e.Feed)
			}
		}
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenNewsFeeds, Result: NewsFeedsResult{Feeds: feeds}}
		}
	case tea.KeyRunes:
		switch key.String() {
		case "a":
			m.mode = nfModeAdd
			m.addStep = nfAddStepName
			m.addErr = ""
			m.nameInput.SetValue("")
			m.urlInput.SetValue("")
			m.langInput.SetValue("")
			m.urlInput.Blur()
			m.langInput.Blur()
			m.nameInput.Focus()
			return m, textinput.Blink
		case "d":
			if len(m.entries) > 0 && !m.entries[m.cursor].IsDefault {
				url := m.entries[m.cursor].Feed.URL
				m.entries = append(m.entries[:m.cursor], m.entries[m.cursor+1:]...)
				delete(m.selected, url)
				if m.cursor >= len(m.entries) && m.cursor > 0 {
					m.cursor--
				}
				m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
			}
		}
	}
	return m, nil
}

// handleAddKey handles keystrokes while filling in the "add a custom feed" form.
func (m *NewsFeedsModel) handleAddKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.addStep {
	case nfAddStepName:
		switch key.Type {
		case tea.KeyEsc:
			m.mode = nfModeList
			m.addErr = ""
			return m, nil
		case tea.KeyEnter:
			if strings.TrimSpace(m.nameInput.Value()) == "" {
				m.addErr = locale.T("newsfeeds.add.name.error_empty")
				return m, nil
			}
			m.addErr = ""
			m.addStep = nfAddStepURL
			m.nameInput.Blur()
			m.urlInput.Focus()
			return m, textinput.Blink
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(key)
		return m, cmd

	case nfAddStepURL:
		switch key.Type {
		case tea.KeyEsc:
			m.addErr = ""
			m.addStep = nfAddStepName
			m.urlInput.Blur()
			m.nameInput.Focus()
			return m, textinput.Blink
		case tea.KeyEnter:
			url := strings.TrimSpace(m.urlInput.Value())
			if url == "" || !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
				m.addErr = locale.T("newsfeeds.add.url.error_invalid")
				return m, nil
			}
			m.addErr = ""
			m.addStep = nfAddStepLanguage
			m.urlInput.Blur()
			m.langInput.Focus()
			return m, textinput.Blink
		}
		var cmd tea.Cmd
		m.urlInput, cmd = m.urlInput.Update(key)
		return m, cmd

	case nfAddStepLanguage:
		switch key.Type {
		case tea.KeyEsc:
			m.addErr = ""
			m.addStep = nfAddStepURL
			m.langInput.Blur()
			m.urlInput.Focus()
			return m, textinput.Blink
		case tea.KeyEnter:
			feed := news.FeedConfig{
				Name:     strings.TrimSpace(m.nameInput.Value()),
				URL:      strings.TrimSpace(m.urlInput.Value()),
				Language: strings.TrimSpace(m.langInput.Value()),
			}
			m.entries = append(m.entries, newsFeedEntry{Feed: feed, IsDefault: false})
			m.selected[feed.URL] = true
			m.cursor = len(m.entries) - 1
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
			m.mode = nfModeList
			m.addErr = ""
			return m, nil
		}
		var cmd tea.Cmd
		m.langInput, cmd = m.langInput.Update(key)
		return m, cmd
	}
	return m, nil
}

// View implements [tea.Model].
func (m *NewsFeedsModel) View() string {
	if m.mode == nfModeAdd {
		return m.viewAdd()
	}
	return m.viewList()
}

func (m *NewsFeedsModel) viewList() string {
	title := m.styles.Title.Render(locale.T("newsfeeds.title"))

	maxVis := m.maxVisible()
	end := m.scrollOff + maxVis
	if end > len(m.entries) {
		end = len(m.entries)
	}

	var rows []string
	if m.scrollOff > 0 {
		rows = append(rows, m.styles.Hint.Render(locale.T("setup.ai.model.scroll_up")))
	}
	for i := m.scrollOff; i < end; i++ {
		e := m.entries[i]
		box := "[ ]"
		if m.selected[e.Feed.URL] {
			box = "[x]"
		}
		checkbox := m.styles.Checkbox.Render(box)
		cursor := "  "
		label := fmt.Sprintf("%s (%s)", e.Feed.Name, e.Feed.Language)
		if !e.IsDefault {
			label = fmt.Sprintf("%s %s", label, locale.T("newsfeeds.custom_tag"))
		}
		style := m.styles.Unselected
		if i == m.cursor {
			cursor = m.styles.Cursor.Render(">")
			style = m.styles.Selected
		}
		rows = append(rows, fmt.Sprintf("%s %s %s", cursor, checkbox, style.Render(label)))
	}
	if end < len(m.entries) {
		rows = append(rows, m.styles.Hint.Render(locale.T("setup.ai.model.scroll_down")))
	}

	hintText := locale.T("newsfeeds.hint")
	if m.canGoBack {
		hintText += "  " + locale.T("hint.esc_back")
	}
	hint := m.styles.Hint.Render(hintText)

	parts := append([]string{title}, rows...)
	parts = append(parts, "", hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *NewsFeedsModel) viewAdd() string {
	title := m.styles.Subtitle.Render(locale.T("newsfeeds.add.title"))

	var body []string
	switch m.addStep {
	case nfAddStepName:
		label := m.styles.Subtitle.Render(locale.T("newsfeeds.add.name.label"))
		input := m.styles.Input.Render(m.nameInput.View())
		hint := m.styles.Hint.Render(locale.T("newsfeeds.add.name.hint"))
		body = append(body, label, input, hint)
	case nfAddStepURL:
		label := m.styles.Subtitle.Render(locale.T("newsfeeds.add.url.label"))
		input := m.styles.Input.Render(m.urlInput.View())
		hint := m.styles.Hint.Render(locale.T("newsfeeds.add.url.hint"))
		body = append(body, label, input, hint)
	case nfAddStepLanguage:
		label := m.styles.Subtitle.Render(locale.T("newsfeeds.add.language.label"))
		input := m.styles.Input.Render(m.langInput.View())
		hint := m.styles.Hint.Render(locale.T("newsfeeds.add.language.hint"))
		body = append(body, label, input, hint)
	}

	parts := []string{title, ""}
	parts = append(parts, body...)
	if m.addErr != "" {
		parts = append(parts, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.addErr)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
