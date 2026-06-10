package main

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/maruel/natural"
)

type candidateItem struct {
	MangaListItem
	tags []Tag
}

func (s *Store) SimilarManga(mhash string, limit int) ([]MangaListItem, error) {
	target, err := s.GetManga(mhash)
	if err != nil {
		return []MangaListItem{}, nil
	}

	candidates, err := s.gatherSimilarCandidates(target, 500)
	if err != nil {
		return nil, err
	}

	type scored struct {
		item  MangaListItem
		score float64
	}
	var items []scored
	for _, c := range candidates {
		if sc := scoreCandidate(target, c); sc > 0 {
			items = append(items, scored{c.MangaListItem, sc})
		}
	}

	slices.SortFunc(items, func(a, b scored) int {
		return cmp.Compare(b.score, a.score) // descending
	})
	if len(items) > limit {
		items = items[:limit]
	}

	result := make([]MangaListItem, len(items))
	for i, it := range items {
		result[i] = it.item
	}
	slices.SortFunc(result, func(a, b MangaListItem) int {
		return natural.Compare(a.Title, b.Title)
	})
	return result, nil
}

func (s *Store) gatherSimilarCandidates(target *MangaDetail, max int) ([]candidateItem, error) {
	var parts []string

	for _, t := range target.Tags {
		switch t.Type {
		case "artist", "parody", "character":
			col := ftsColFor(t.Type)
			parts = append(parts, col+" : "+escapeFTSValue(t.Name))
		}
	}

	words := titleWords(target.Title)
	for i, w := range words {
		if i >= 3 {
			break
		}
		parts = append(parts, "title : "+escapeFTSValue(w))
	}

	if len(parts) == 0 {
		return nil, nil
	}

	ftsQuery := strings.Join(parts, " OR ")

	rows, err := s.db.Query(`
		SELECT m.mhash, m.title, m.mtime,
		       COALESCE(json_extract(m.metadata,'$.page_count'),0),
		       m.metadata
		FROM search s
		JOIN manga m ON m.mhash=s.mhash
		WHERE s.mhash != ? AND search MATCH ?
		LIMIT ?`, target.Mhash, ftsQuery, max)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var out []candidateItem
	for rows.Next() {
		var c candidateItem
		var metaJSON string
		if err := rows.Scan(&c.Mhash, &c.Title, &c.Mtime, &c.PageCount, &metaJSON); err != nil {
			return nil, err
		}
		if seen[c.Mhash] {
			continue
		}
		seen[c.Mhash] = true
		c.tags = tagsFromMetadataJSON(metaJSON)
		out = append(out, c)
	}
	return out, rows.Err()
}

var tagWeights = map[string]float64{
	"artist":    5.0,
	"parody":    3.0,
	"character": 2.0,
	"group":     1.5,
	"category":  1.0,
	"tag":       1.0,
	"language":  0.5,
}

func scoreCandidate(target *MangaDetail, c candidateItem) float64 {
	targetSet := make(map[string]bool, len(target.Tags))
	for _, t := range target.Tags {
		targetSet[t.Type+":"+t.Name] = true
	}

	var score float64
	for _, t := range c.tags {
		if targetSet[t.Type+":"+t.Name] {
			w := tagWeights[t.Type]
			if w == 0 {
				w = 1.0
			}
			score += w
		}
	}

	tgt := makeTrigrams(target.Title)
	cnd := makeTrigrams(c.Title)
	score += trigramJaccard(tgt, cnd) * 3.0

	return score
}

func makeTrigrams(s string) map[string]struct{} {
	// normalize: lowercase, collapse non-alphanumeric runs to a single space
	var b strings.Builder
	prevSpace := true
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
		} else if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	norm := strings.TrimSpace(b.String())

	out := make(map[string]struct{})
	runes := []rune(norm)
	for i := 0; i+2 < len(runes); i++ {
		out[string(runes[i:i+3])] = struct{}{}
	}
	return out
}

func trigramJaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	for k := range a {
		if _, ok := b[k]; ok {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// titleWords returns lowercase words of 3+ runes, capped at a reasonable count.
func titleWords(title string) []string {
	var words []string
	for _, w := range strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len([]rune(w)) >= 3 {
			words = append(words, w)
		}
	}
	return words
}
