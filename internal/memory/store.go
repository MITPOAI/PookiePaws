package memory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, path: path}, nil
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Initialize(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("memory store is not open")
	}
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM brand_profile WHERE id = 1`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return s.SaveBrandProfile(ctx, DefaultBrandProfile())
	}
	return nil
}

func (s *Store) GetBrandProfile(ctx context.Context) (BrandProfile, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT brand_name, niche, colors_json, fonts_json, tone, target_audience,
       preferred_video_style, preferred_cta_style, banned_words_json,
       banned_styles_json, successful_prompts_json, failed_prompts_json,
       platform_preferences_json, created_at, updated_at
FROM brand_profile WHERE id = 1`)
	var profile BrandProfile
	var colors, fonts, bannedWords, bannedStyles, successful, failed, prefs string
	var createdAt, updatedAt string
	if err := row.Scan(
		&profile.BrandName,
		&profile.Niche,
		&colors,
		&fonts,
		&profile.Tone,
		&profile.TargetAudience,
		&profile.PreferredVideoStyle,
		&profile.PreferredCTAStyle,
		&bannedWords,
		&bannedStyles,
		&successful,
		&failed,
		&prefs,
		&createdAt,
		&updatedAt,
	); err != nil {
		return BrandProfile{}, err
	}
	if err := decodeJSON(colors, &profile.Colors); err != nil {
		return BrandProfile{}, err
	}
	if err := decodeJSON(fonts, &profile.Fonts); err != nil {
		return BrandProfile{}, err
	}
	if err := decodeJSON(bannedWords, &profile.BannedWords); err != nil {
		return BrandProfile{}, err
	}
	if err := decodeJSON(bannedStyles, &profile.BannedStyles); err != nil {
		return BrandProfile{}, err
	}
	if err := decodeJSON(successful, &profile.SuccessfulPastPrompts); err != nil {
		return BrandProfile{}, err
	}
	if err := decodeJSON(failed, &profile.FailedPastPrompts); err != nil {
		return BrandProfile{}, err
	}
	if err := decodeJSON(prefs, &profile.PlatformPreferences); err != nil {
		return BrandProfile{}, err
	}
	profile.CreatedAt = parseTime(createdAt)
	profile.UpdatedAt = parseTime(updatedAt)
	if profile.PlatformPreferences == nil {
		profile.PlatformPreferences = map[string]PlatformPreference{}
	}
	return profile, nil
}

func (s *Store) SaveBrandProfile(ctx context.Context, profile BrandProfile) error {
	now := time.Now().UTC()
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	profile.UpdatedAt = now
	if profile.PlatformPreferences == nil {
		profile.PlatformPreferences = map[string]PlatformPreference{}
	}
	profile.Colors = nonNilStrings(profile.Colors)
	profile.Fonts = nonNilStrings(profile.Fonts)
	profile.BannedWords = nonNilStrings(profile.BannedWords)
	profile.BannedStyles = nonNilStrings(profile.BannedStyles)
	profile.SuccessfulPastPrompts = nonNilStrings(profile.SuccessfulPastPrompts)
	profile.FailedPastPrompts = nonNilStrings(profile.FailedPastPrompts)

	_, err := s.db.ExecContext(ctx, `
INSERT INTO brand_profile (
  id, brand_name, niche, colors_json, fonts_json, tone, target_audience,
  preferred_video_style, preferred_cta_style, banned_words_json, banned_styles_json,
  successful_prompts_json, failed_prompts_json, platform_preferences_json, created_at, updated_at
) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  brand_name = excluded.brand_name,
  niche = excluded.niche,
  colors_json = excluded.colors_json,
  fonts_json = excluded.fonts_json,
  tone = excluded.tone,
  target_audience = excluded.target_audience,
  preferred_video_style = excluded.preferred_video_style,
  preferred_cta_style = excluded.preferred_cta_style,
  banned_words_json = excluded.banned_words_json,
  banned_styles_json = excluded.banned_styles_json,
  successful_prompts_json = excluded.successful_prompts_json,
  failed_prompts_json = excluded.failed_prompts_json,
  platform_preferences_json = excluded.platform_preferences_json,
  updated_at = excluded.updated_at`,
		profile.BrandName,
		profile.Niche,
		mustJSON(profile.Colors),
		mustJSON(profile.Fonts),
		profile.Tone,
		profile.TargetAudience,
		profile.PreferredVideoStyle,
		profile.PreferredCTAStyle,
		mustJSON(profile.BannedWords),
		mustJSON(profile.BannedStyles),
		mustJSON(profile.SuccessfulPastPrompts),
		mustJSON(profile.FailedPastPrompts),
		mustJSON(profile.PlatformPreferences),
		formatTime(profile.CreatedAt),
		formatTime(profile.UpdatedAt),
	)
	return err
}

func (s *Store) SaveProject(ctx context.Context, project ProjectHistory) error {
	if strings.TrimSpace(project.ID) == "" {
		project.ID = newID("project")
	}
	if project.CreatedAt.IsZero() {
		project.CreatedAt = time.Now().UTC()
	}
	var score any
	if project.FeedbackScore != nil {
		score = *project.FeedbackScore
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO project_history (
  id, created_at, user_request, platform, duration_sec, provider, generated_brief,
  prompts_json, model_used, edit_plan_path, final_output_path, review_report_path,
  feedback_score, user_corrections, lessons_learned
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  user_request = excluded.user_request,
  platform = excluded.platform,
  duration_sec = excluded.duration_sec,
  provider = excluded.provider,
  generated_brief = excluded.generated_brief,
  prompts_json = excluded.prompts_json,
  model_used = excluded.model_used,
  edit_plan_path = excluded.edit_plan_path,
  final_output_path = excluded.final_output_path,
  review_report_path = excluded.review_report_path,
  feedback_score = excluded.feedback_score,
  user_corrections = excluded.user_corrections,
  lessons_learned = excluded.lessons_learned`,
		project.ID,
		formatTime(project.CreatedAt),
		project.UserRequest,
		project.Platform,
		project.DurationSec,
		project.Provider,
		project.GeneratedBrief,
		mustJSON(project.PromptsUsed),
		project.ModelUsed,
		project.EditPlanPath,
		project.FinalOutputPath,
		project.ReviewReportPath,
		score,
		project.UserCorrections,
		project.LessonsLearned,
	)
	return err
}

func (s *Store) GetProject(ctx context.Context, id string) (ProjectHistory, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, created_at, user_request, platform, duration_sec, provider, generated_brief,
       prompts_json, model_used, edit_plan_path, final_output_path, review_report_path,
       feedback_score, user_corrections, lessons_learned
FROM project_history WHERE id = ?`, id)
	if err != nil {
		return ProjectHistory{}, err
	}
	defer rows.Close()
	projects, err := scanProjects(rows)
	if err != nil {
		return ProjectHistory{}, err
	}
	if len(projects) == 0 {
		return ProjectHistory{}, sql.ErrNoRows
	}
	return projects[0], nil
}

func (s *Store) ListProjects(ctx context.Context, limit int) ([]ProjectHistory, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, created_at, user_request, platform, duration_sec, provider, generated_brief,
       prompts_json, model_used, edit_plan_path, final_output_path, review_report_path,
       feedback_score, user_corrections, lessons_learned
FROM project_history
ORDER BY created_at DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjects(rows)
}

func (s *Store) AddFeedback(ctx context.Context, projectID string, score int, corrections, lessons string) (Feedback, error) {
	if score < 1 || score > 5 {
		return Feedback{}, errors.New("feedback score must be between 1 and 5")
	}
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return Feedback{}, fmt.Errorf("find project: %w", err)
	}

	item := Feedback{
		ID:              newID("feedback"),
		ProjectID:       projectID,
		CreatedAt:       time.Now().UTC(),
		Score:           score,
		UserCorrections: strings.TrimSpace(corrections),
		LessonsLearned:  strings.TrimSpace(lessons),
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Feedback{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO feedback (id, project_id, created_at, score, user_corrections, lessons_learned)
VALUES (?, ?, ?, ?, ?, ?)`,
		item.ID,
		item.ProjectID,
		formatTime(item.CreatedAt),
		item.Score,
		item.UserCorrections,
		item.LessonsLearned,
	); err != nil {
		return Feedback{}, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE project_history
SET feedback_score = ?, user_corrections = ?, lessons_learned = ?
WHERE id = ?`,
		item.Score,
		item.UserCorrections,
		item.LessonsLearned,
		item.ProjectID,
	); err != nil {
		return Feedback{}, err
	}
	if err := tx.Commit(); err != nil {
		return Feedback{}, err
	}

	if err := s.learnFromFeedback(ctx, project, item); err != nil {
		return Feedback{}, err
	}
	return item, nil
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	like := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx, `
SELECT kind, id, created_at, summary FROM (
  SELECT 'project' AS kind, id, created_at,
         user_request || ' | ' || generated_brief AS summary
  FROM project_history
  WHERE lower(user_request || ' ' || generated_brief || ' ' || prompts_json || ' ' || user_corrections || ' ' || lessons_learned) LIKE ?
  UNION ALL
  SELECT 'feedback' AS kind, id, created_at,
         user_corrections || ' | ' || lessons_learned AS summary
  FROM feedback
  WHERE lower(user_corrections || ' ' || lessons_learned) LIKE ?
) ORDER BY created_at DESC
LIMIT ?`, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var item SearchResult
		var createdAt string
		if err := rows.Scan(&item.Kind, &item.ID, &createdAt, &item.Summary); err != nil {
			return nil, err
		}
		item.CreatedAt = parseTime(createdAt)
		results = append(results, item)
	}
	return results, rows.Err()
}

func (s *Store) Export(ctx context.Context, w io.Writer) error {
	profile, err := s.GetBrandProfile(ctx)
	if err != nil {
		return err
	}
	projects, err := s.ListProjects(ctx, 1000)
	if err != nil {
		return err
	}
	feedbackRows, err := s.listFeedback(ctx, 1000)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{
		"brand_profile": profile,
		"projects":      projects,
		"feedback":      feedbackRows,
	})
}

func (s *Store) Reset(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		`DELETE FROM feedback`,
		`DELETE FROM project_history`,
		`DELETE FROM brand_profile`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.SaveBrandProfile(ctx, DefaultBrandProfile())
}

func (s *Store) learnFromFeedback(ctx context.Context, project ProjectHistory, item Feedback) error {
	profile, err := s.GetBrandProfile(ctx)
	if err != nil {
		return err
	}
	if item.Score >= 4 {
		profile.SuccessfulPastPrompts = appendUnique(profile.SuccessfulPastPrompts, project.PromptsUsed...)
	} else {
		profile.FailedPastPrompts = appendUnique(profile.FailedPastPrompts, project.PromptsUsed...)
	}
	if item.LessonsLearned != "" {
		key := strings.ToLower(strings.TrimSpace(project.Platform))
		if key == "" {
			key = "general"
		}
		if profile.PlatformPreferences == nil {
			profile.PlatformPreferences = map[string]PlatformPreference{}
		}
		pref := profile.PlatformPreferences[key]
		pref.Lessons = appendUnique(pref.Lessons, item.LessonsLearned)
		profile.PlatformPreferences[key] = pref
	}
	return s.SaveBrandProfile(ctx, profile)
}

func (s *Store) listFeedback(ctx context.Context, limit int) ([]Feedback, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, project_id, created_at, score, user_corrections, lessons_learned
FROM feedback
ORDER BY created_at DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Feedback
	for rows.Next() {
		var item Feedback
		var createdAt string
		if err := rows.Scan(&item.ID, &item.ProjectID, &createdAt, &item.Score, &item.UserCorrections, &item.LessonsLearned); err != nil {
			return nil, err
		}
		item.CreatedAt = parseTime(createdAt)
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanProjects(rows *sql.Rows) ([]ProjectHistory, error) {
	var projects []ProjectHistory
	for rows.Next() {
		var project ProjectHistory
		var createdAt, prompts string
		var score sql.NullInt64
		if err := rows.Scan(
			&project.ID,
			&createdAt,
			&project.UserRequest,
			&project.Platform,
			&project.DurationSec,
			&project.Provider,
			&project.GeneratedBrief,
			&prompts,
			&project.ModelUsed,
			&project.EditPlanPath,
			&project.FinalOutputPath,
			&project.ReviewReportPath,
			&score,
			&project.UserCorrections,
			&project.LessonsLearned,
		); err != nil {
			return nil, err
		}
		project.CreatedAt = parseTime(createdAt)
		if score.Valid {
			v := int(score.Int64)
			project.FeedbackScore = &v
		}
		if err := decodeJSON(prompts, &project.PromptsUsed); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func appendUnique(base []string, values ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(values))
	out := make([]string, 0, len(base)+len(values))
	for _, value := range append(base, values...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

func decodeJSON(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		raw = "null"
	}
	return json.Unmarshal([]byte(raw), target)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func newID(prefix string) string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf[:])
}

const schemaSQL = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS brand_profile (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  brand_name TEXT NOT NULL,
  niche TEXT NOT NULL,
  colors_json TEXT NOT NULL,
  fonts_json TEXT NOT NULL,
  tone TEXT NOT NULL,
  target_audience TEXT NOT NULL,
  preferred_video_style TEXT NOT NULL,
  preferred_cta_style TEXT NOT NULL,
  banned_words_json TEXT NOT NULL,
  banned_styles_json TEXT NOT NULL,
  successful_prompts_json TEXT NOT NULL,
  failed_prompts_json TEXT NOT NULL,
  platform_preferences_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_history (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  user_request TEXT NOT NULL,
  platform TEXT NOT NULL,
  duration_sec INTEGER NOT NULL,
  provider TEXT NOT NULL,
  generated_brief TEXT NOT NULL,
  prompts_json TEXT NOT NULL,
  model_used TEXT NOT NULL,
  edit_plan_path TEXT NOT NULL,
  final_output_path TEXT NOT NULL,
  review_report_path TEXT NOT NULL,
  feedback_score INTEGER,
  user_corrections TEXT NOT NULL DEFAULT '',
  lessons_learned TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS feedback (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  score INTEGER NOT NULL CHECK(score BETWEEN 1 AND 5),
  user_corrections TEXT NOT NULL DEFAULT '',
  lessons_learned TEXT NOT NULL DEFAULT '',
  FOREIGN KEY(project_id) REFERENCES project_history(id) ON DELETE CASCADE
);
`
