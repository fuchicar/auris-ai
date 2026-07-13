package simulation

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"auris/pkg/news"
)

// NewsSource implements news.Source with synthetic headlines derived from
// this driver's simulated price moves, instead of real RSS feeds — there is
// no real news for fabricated companies. Positive/negative headline
// templates are picked deterministically per (symbol, day), same as every
// other generator output, so a demo session's news stays consistent.
type NewsSource struct {
	driver *Driver
}

var positiveHeadlines = []string{
	"%s sube un %.1f%% tras superar las previsiones de beneficio",
	"%s se dispara un %.1f%% después de un informe de resultados mejor de lo esperado",
	"Los analistas elevan el precio objetivo de %s tras un salto del %.1f%%",
	"%s avanza un %.1f%% impulsada por una demanda mejor de lo previsto",
}

var negativeHeadlines = []string{
	"%s cae un %.1f%% tras recortar sus previsiones",
	"%s se desploma un %.1f%% ante la debilidad de la demanda",
	"%s retrocede un %.1f%% después de unos resultados decepcionantes",
	"Los analistas rebajan la recomendación de %s tras una caída del %.1f%%",
}

// bigMoveThreshold is the minimum absolute daily return that becomes a
// headline — small day-to-day noise doesn't generate news.
const bigMoveThreshold = 0.02

// HandleFetchNews implements news.Source.
func (n *NewsSource) HandleFetchNews(_ context.Context, params news.FetchNewsParams) ([]news.NewsItem, error) {
	gen, err := n.driver.generator()
	if err != nil {
		return nil, fmt.Errorf("simulation: HandleFetchNews: %w", err)
	}

	maxAge := params.MaxAgeHours
	if maxAge == 0 {
		maxAge = 48
	}
	maxResults := params.MaxResults
	if maxResults == 0 {
		maxResults = 10
	}

	keywords := make([]string, 0, len(params.Keywords))
	for _, k := range params.Keywords {
		keywords = append(keywords, strings.ToLower(k))
	}

	now := time.Now().UTC()
	since := now.Add(-time.Duration(maxAge) * time.Hour)

	var items []news.NewsItem
	for _, inst := range gen.instruments {
		if !matchesKeywords(inst, keywords) {
			continue
		}
		series := gen.closeSeries(inst, now)
		for i := len(series) - 1; i >= 0 && series[i].date.After(since); i-- {
			d := series[i]
			if math.Abs(d.ret) < bigMoveThreshold {
				continue
			}
			items = append(items, newsItemForMove(inst, d))
		}
	}

	if len(items) == 0 && len(keywords) == 0 {
		items = append(items, genericFillerHeadline(now))
	}

	sort.Slice(items, func(i, j int) bool { return items[i].PublishedAt.After(items[j].PublishedAt) })
	if len(items) > maxResults {
		items = items[:maxResults]
	}
	return items, nil
}

func matchesKeywords(inst instrumentDef, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	name := strings.ToLower(inst.Name)
	sym := strings.ToLower(inst.Symbol)
	for _, k := range keywords {
		if strings.Contains(name, k) || strings.Contains(sym, k) {
			return true
		}
	}
	return false
}

func newsItemForMove(inst instrumentDef, d dayClose) news.NewsItem {
	r := deterministicRand(inst.Symbol, d.date.Format(time.DateOnly), "headline")
	pctMove := d.ret * 100

	var tmpl string
	if d.ret >= 0 {
		tmpl = positiveHeadlines[r.IntN(len(positiveHeadlines))]
	} else {
		tmpl = negativeHeadlines[r.IntN(len(negativeHeadlines))]
	}
	title := fmt.Sprintf(tmpl, inst.Name, math.Abs(pctMove))

	return news.NewsItem{
		Title:       title,
		Summary:     title,
		Source:      "Auris Simulación",
		URL:         fmt.Sprintf("simulation://news/%s/%s", inst.Symbol, d.date.Format(time.DateOnly)),
		PublishedAt: d.date.Add(15 * time.Hour),
	}
}

func genericFillerHeadline(now time.Time) news.NewsItem {
	const text = "Los mercados simulados se mantienen estables ante la falta de catalizadores relevantes"
	return news.NewsItem{
		Title:       text,
		Summary:     "Sin movimientos relevantes en las últimas horas en el universo de datos simulados.",
		Source:      "Auris Simulación",
		URL:         "simulation://news/market/overview",
		PublishedAt: now,
	}
}
