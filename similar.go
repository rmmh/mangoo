package main

import (
	"cmp"
	"fmt"
	"log/slog"
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

	scores := make([]float64, len(items))
	for i, it := range items {
		scores[i] = it.score
	}

	top5 := items
	if len(top5) > 5 {
		top5 = top5[:5]
	}
	slog.Debug("similar top5", "mhash", mhash, "total_scored", len(items),
		"top5", func() []string {
			out := make([]string, len(top5))
			for i, it := range top5 {
				out[i] = fmt.Sprintf("%s %.2f", it.item.Mhash, it.score)
			}
			return out
		}())


	items = items[:dynamicCutoff(scores, limit)]

	// Nudge scores toward natural (alphabetical) order without overriding relevance.
	// Items that would appear earlier in natural order get +20%; later get -20%.
	if len(items) > 1 {
		naturalOrder := make([]int, len(items))
		for i := range naturalOrder {
			naturalOrder[i] = i
		}
		slices.SortFunc(naturalOrder, func(a, b int) int {
			return natural.Compare(items[a].item.Title, items[b].item.Title)
		})
		naturalPos := make([]int, len(items))
		for naturalIdx, scoreIdx := range naturalOrder {
			naturalPos[scoreIdx] = naturalIdx
		}
		for i := range items {
			if naturalPos[i] < i {
				items[i].score *= 1.2
			} else if naturalPos[i] > i {
				items[i].score *= 0.8
			}
		}
		slices.SortFunc(items, func(a, b scored) int {
			return cmp.Compare(b.score, a.score)
		})
	}

	result := make([]MangaListItem, len(items))
	for i, it := range items {
		result[i] = it.item
	}
	return result, nil
}

func (s *Store) gatherSimilarCandidates(target *MangaDetail, max int) ([]candidateItem, error) {
	seen := make(map[string]bool)
	var out []candidateItem

	// Pass 1: artist-only scan — artists are the strongest similarity signal.
	var artistParts []string
	for _, t := range target.Tags {
		if t.Type == "artist" {
			artistParts = append(artistParts, ftsColFor("artist")+" : "+escapeFTSValue(t.Name))
		}
	}
	if len(artistParts) > 0 {
		if err := s.scanFTSCandidates(target.Mhash, strings.Join(artistParts, " OR "), max, seen, &out); err != nil {
			return nil, err
		}
	}

	// Pass 2: general scan — parody, character, and title words.
	var parts []string
	for _, t := range target.Tags {
		switch t.Type {
		case "parody", "character":
			parts = append(parts, ftsColFor(t.Type)+" : "+escapeFTSValue(t.Name))
		}
	}
	words := titleWords(target.Title)
	for i, w := range words {
		if i >= 3 {
			break
		}
		parts = append(parts, "title : "+escapeFTSValue(w))
	}
	if len(parts) > 0 {
		if err := s.scanFTSCandidates(target.Mhash, strings.Join(parts, " OR "), max, seen, &out); err != nil {
			return nil, err
		}
	}

	return out, nil
}

func (s *Store) scanFTSCandidates(excludeMhash, ftsQuery string, limit int, seen map[string]bool, out *[]candidateItem) error {
	rows, err := s.db.Query(`
		SELECT m.mhash, m.title, m.mtime,
		       COALESCE(json_extract(m.metadata,'$.page_count'),0),
		       m.metadata
		FROM search s
		JOIN manga m ON m.mhash=s.mhash
		WHERE s.mhash != ? AND search MATCH ?
		LIMIT ?`, excludeMhash, ftsQuery, limit)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var c candidateItem
		var metaJSON string
		if err := rows.Scan(&c.Mhash, &c.Title, &c.Mtime, &c.PageCount, &metaJSON); err != nil {
			return err
		}
		if seen[c.Mhash] {
			continue
		}
		seen[c.Mhash] = true
		c.tags = tagsFromMetadataJSON(metaJSON)
		*out = append(*out, c)
	}
	return rows.Err()
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

// dynamicCutoff picks a natural end index for a descending score slice.
//
// It first trims items below a floor (the greater of 1.5 points or 15% of
// the top score), then cuts at the first "cliff" — a step where the score
// drops by half or more relative to the previous item. hardMax is the
// absolute upper bound regardless of the curve.
func dynamicCutoff(scores []float64, hardMax int) int {
	n := min(len(scores), hardMax)
	if n == 0 {
		return 0
	}
	top := scores[0]
	if top == 0 {
		return 0
	}

	// Score floor: at least 15% of the best score.
	floor := top * 0.15
	for i := range n {
		if scores[i] < floor {
			n = i
			break
		}
	}
	if n == 0 {
		return 0
	}

	// First cliff: score falls to half or less of the previous item's score.
	for i := 1; i < n; i++ {
		if scores[i] <= scores[i-1]*0.5 {
			return i
		}
	}
	return n
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
