package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func openStore(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=auto_vacuum(INCREMENTAL)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	s := &Store{db: db}
	return s, s.initSchema()
}

func (s *Store) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS file (
			path  TEXT PRIMARY KEY,
			mtime INTEGER NOT NULL,
			size  INTEGER NOT NULL,
			mhash TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_file_mhash ON file(mhash);

		CREATE TABLE IF NOT EXISTS manga (
			mhash    TEXT PRIMARY KEY,
			title    TEXT NOT NULL,
			mtime    INTEGER NOT NULL,
			metadata TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_manga_mtime ON manga(mtime);
		CREATE INDEX IF NOT EXISTS idx_manga_title ON manga(title);

		CREATE TABLE IF NOT EXISTS thumbnail (
			mhash TEXT PRIMARY KEY,
			data  BLOB NOT NULL
		);

		CREATE VIRTUAL TABLE IF NOT EXISTS search USING fts5(
			mhash UNINDEXED,
			title,
			artist,
			category,
			character,
			group_col,
			language,
			parody,
			tag,
			tags,
			tokenize='trigram'
		);
	`)
	return err
}

// --- file table ---

func (s *Store) AllFilePaths() (map[string]struct{}, error) {
	rows, err := s.db.Query(`SELECT path FROM file`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]struct{})
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		m[p] = struct{}{}
	}
	return m, rows.Err()
}

func (s *Store) GetFileMtimeSize(path string) (mtime, size int64, found bool, err error) {
	err = s.db.QueryRow(`SELECT mtime, size FROM file WHERE path=?`, path).Scan(&mtime, &size)
	if err == sql.ErrNoRows {
		return 0, 0, false, nil
	}
	return mtime, size, err == nil, err
}

func (s *Store) UpsertFile(path string, mtime, size int64, mhash string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO file(path,mtime,size,mhash) VALUES(?,?,?,?)`,
		path, mtime, size, mhash,
	)
	return err
}

type UpsertRecord struct {
	Path, Mhash, Title, MetadataJSON string
	Mtime, Size                      int64
}

func (s *Store) UpsertBatch(records []UpsertRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, r := range records {
		if _, err = tx.Exec(
			`INSERT OR REPLACE INTO file(path,mtime,size,mhash) VALUES(?,?,?,?)`,
			r.Path, r.Mtime, r.Size, r.Mhash,
		); err != nil {
			return err
		}
		if _, err = tx.Exec(`
			INSERT INTO manga(mhash,title,mtime,metadata) VALUES(?,?,?,?)
			ON CONFLICT(mhash) DO UPDATE SET title=excluded.title, metadata=excluded.metadata`,
			r.Mhash, r.Title, r.Mtime, r.MetadataJSON,
		); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM search WHERE mhash=?`, r.Mhash); err != nil {
			return err
		}
		var fts ftsFields
		fts.fromMetadataJSON(r.Title, r.MetadataJSON)
		if _, err = tx.Exec(`INSERT INTO search(mhash,title,artist,category,character,group_col,language,parody,tag,tags)
			VALUES(?,?,?,?,?,?,?,?,?,?)`,
			r.Mhash, fts.title, fts.artist, fts.category, fts.character,
			fts.group, fts.language, fts.parody, fts.tag, fts.tags,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) DeleteFiles(paths []string) error {
	for chunk := range slices.Chunk(paths, 500) {
		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]any, len(chunk))
		for j, p := range chunk {
			args[j] = p
		}
		if _, err := s.db.Exec(`DELETE FROM file WHERE path IN (`+placeholders+`)`, args...); err != nil {
			return err
		}
	}
	return nil
}

// --- manga table ---

func (s *Store) PruneOrphanManga() error {
	_, err := s.db.Exec(`DELETE FROM manga WHERE mhash NOT IN (SELECT mhash FROM file)`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM search WHERE mhash NOT IN (SELECT mhash FROM manga)`)
	return err
}

func (s *Store) PruneOrphanThumbnails() error {
	_, err := s.db.Exec(`DELETE FROM thumbnail WHERE mhash NOT IN (SELECT mhash FROM manga)`)
	return err
}

// --- thumbnail table ---

type ThumbnailRow struct {
	Mhash string
	Data  []byte
}

func (s *Store) MhashesWithoutThumbnails() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT m.mhash FROM manga m
		LEFT JOIN thumbnail t ON m.mhash=t.mhash
		WHERE t.mhash IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) GetFilePathForMhash(mhash string) (string, error) {
	var path string
	err := s.db.QueryRow(`SELECT path FROM file WHERE mhash=? LIMIT 1`, mhash).Scan(&path)
	return path, err
}

func (s *Store) InsertThumbnails(batch []ThumbnailRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO thumbnail(mhash,data) VALUES(?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, row := range batch {
		if _, err := stmt.Exec(row.Mhash, row.Data); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetThumbnail(mhash string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM thumbnail WHERE mhash=?`, mhash).Scan(&data)
	return data, err
}

// --- API queries ---

type MangaListItem struct {
	Mhash     string `json:"mhash"`
	Title     string `json:"title"`
	Mtime     int64  `json:"mtime"`
	PageCount int    `json:"page_count"`
}

type Tag struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type MangaDetail struct {
	Mhash     string `json:"mhash"`
	Title     string `json:"title"`
	Mtime     int64  `json:"mtime"`
	PageCount int    `json:"page_count"`
	FilePath  string `json:"file_path"`
	FileSize  int64  `json:"file_size"`
	Tags      []Tag  `json:"tags"`
}

func (s *Store) ListManga(page, perPage int, sortBy string) ([]MangaListItem, int, error) {
	orderBy := "m.mtime DESC"
	if sortBy == "title" {
		orderBy = "m.title ASC"
	}
	offset := (page - 1) * perPage

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT m.mhash, m.title, m.mtime,
		       COALESCE(json_extract(m.metadata,'$.page_count'),0)
		FROM manga m
		ORDER BY %s LIMIT ? OFFSET ?`, orderBy),
		perPage, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []MangaListItem
	for rows.Next() {
		var it MangaListItem
		if err := rows.Scan(&it.Mhash, &it.Title, &it.Mtime, &it.PageCount); err != nil {
			return nil, 0, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int
	s.db.QueryRow(`SELECT COUNT(*) FROM manga`).Scan(&total)
	return items, total, nil
}

func (s *Store) GetManga(mhash string) (*MangaDetail, error) {
	var d MangaDetail
	var metaJSON string
	err := s.db.QueryRow(`
		SELECT m.mhash, m.title, m.mtime,
		       COALESCE(json_extract(m.metadata,'$.page_count'),0),
		       m.metadata,
		       f.path, f.size
		FROM manga m
		JOIN file f ON f.mhash=m.mhash
		WHERE m.mhash=?
		ORDER BY f.rowid DESC LIMIT 1`, mhash).
		Scan(&d.Mhash, &d.Title, &d.Mtime, &d.PageCount, &metaJSON, &d.FilePath, &d.FileSize)
	if err != nil {
		return nil, err
	}
	d.Tags = tagsFromMetadataJSON(metaJSON)
	return &d, nil
}

func (s *Store) RandomManga(q string) (string, error) {
	ftsQuery := buildFTSQuery(q)
	var mhash string
	var err error
	if ftsQuery == "" {
		err = s.db.QueryRow(`SELECT mhash FROM manga ORDER BY RANDOM() LIMIT 1`).Scan(&mhash)
	} else {
		err = s.db.QueryRow(`
			SELECT m.mhash FROM search s
			JOIN manga m ON m.mhash=s.mhash
			WHERE search MATCH ?
			ORDER BY RANDOM() LIMIT 1`, ftsQuery).Scan(&mhash)
	}
	return mhash, err
}

func (s *Store) Search(q string, page, perPage int, sortBy string) ([]MangaListItem, int, error) {
	ftsQuery := buildFTSQuery(q)
	if ftsQuery == "" {
		return s.ListManga(page, perPage, sortBy)
	}

	orderBy := "m.mtime DESC"
	if sortBy == "title" {
		orderBy = "m.title ASC"
	}
	offset := (page - 1) * perPage

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT m.mhash, m.title, m.mtime,
		       COALESCE(json_extract(m.metadata,'$.page_count'),0)
		FROM search s
		JOIN manga m ON m.mhash=s.mhash
		WHERE search MATCH ?
		ORDER BY %s LIMIT ? OFFSET ?`, orderBy),
		ftsQuery, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("fts search: %w (query: %s)", err, ftsQuery)
	}
	defer rows.Close()
	var items []MangaListItem
	for rows.Next() {
		var it MangaListItem
		if err := rows.Scan(&it.Mhash, &it.Title, &it.Mtime, &it.PageCount); err != nil {
			return nil, 0, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int
	s.db.QueryRow(`SELECT COUNT(*) FROM search WHERE search MATCH ?`, ftsQuery).Scan(&total)
	return items, total, nil
}

// --- FTS helpers ---

const ftsAllCols = "{title artist category character group_col language parody tag tags}"

var facetRe = regexp.MustCompile(`^(\w+):"([^"]*)"`)
var facetReUnquoted = regexp.MustCompile(`^(\w+):(\S+)`)

// tagTypeToFTSCol maps metadata tag types to FTS5 column names.
var tagTypeToFTSCol = map[string]string{
	"artist":    "artist",
	"category":  "category",
	"character": "character",
	"group":     "group_col",
	"language":  "language",
	"parody":    "parody",
	"tag":       "tag",
}

func buildFTSQuery(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}

	var freeform []string
	var facets []string

	for q != "" {
		q = strings.TrimLeft(q, " \t")
		if q == "" {
			break
		}
		if m := facetRe.FindStringSubmatch(q); m != nil {
			col := ftsColFor(m[1])
			val := escapeFTSValue(m[2])
			facets = append(facets, col+` : `+val)
			q = q[len(m[0]):]
		} else if m := facetReUnquoted.FindStringSubmatch(q); m != nil {
			col := ftsColFor(m[1])
			val := escapeFTSValue(m[2])
			facets = append(facets, col+` : `+val)
			q = q[len(m[0]):]
		} else {
			// grab next whitespace-delimited token
			idx := strings.IndexAny(q, " \t")
			var token string
			if idx < 0 {
				token, q = q, ""
			} else {
				token, q = q[:idx], q[idx:]
			}
			if len([]rune(token)) >= 3 {
				freeform = append(freeform, escapeFTSValue(token))
			}
		}
	}

	var parts []string
	if len(freeform) > 0 {
		parts = append(parts, ftsAllCols+` : `+strings.Join(freeform, " "))
	}
	parts = append(parts, facets...)
	return strings.Join(parts, " AND ")
}

func ftsColFor(tagType string) string {
	if col, ok := tagTypeToFTSCol[strings.ToLower(tagType)]; ok {
		return col
	}
	return "tags"
}

func escapeFTSValue(s string) string {
	s = strings.ReplaceAll(s, `"`, `""`)
	return `"` + s + `"`
}

// --- metadata JSON helpers ---

type metadataJSON struct {
	PageCount int   `json:"page_count"`
	Tags      []Tag `json:"tags"`
}

type ftsFields struct {
	title, artist, category, character, group, language, parody, tag, tags string
}

func (f *ftsFields) fromMetadataJSON(title, metaJSON string) {
	f.title = title
	var m metadataJSON
	// best-effort parse; zero value on failure
	_ = json.Unmarshal([]byte(metaJSON), &m)
	for _, t := range m.Tags {
		name := t.Name + " "
		f.tags += name
		switch t.Type {
		case "artist":
			f.artist += name
		case "category":
			f.category += name
		case "character":
			f.character += name
		case "group":
			f.group += name
		case "language":
			f.language += name
		case "parody":
			f.parody += name
		case "tag":
			f.tag += name
		}
	}
}

func tagsFromMetadataJSON(metaJSON string) []Tag {
	var m metadataJSON
	_ = json.Unmarshal([]byte(metaJSON), &m)
	if m.Tags == nil {
		return []Tag{}
	}
	return m.Tags
}

func buildMetadataJSON(pageCount int, tags []Tag) (string, error) {
	m := metadataJSON{PageCount: pageCount, Tags: tags}
	if m.Tags == nil {
		m.Tags = []Tag{}
	}
	b, err := json.Marshal(m)
	return string(b), err
}
