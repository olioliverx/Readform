package main

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestParseCaixinWeeklyIndex(t *testing.T) {
	html := `
	<html><body>
	<div><a href="https://weekly.caixin.com/2026/cw1192/">财新周刊第1192期</a><span>2026-02-02</span></div>
	<div><a href="/2026/cw1191/">财新周刊第1191期</a><span>2026年01月26日</span></div>
	<div><a href="/about">About</a></div>
	</body></html>
	`

	issues, err := parseCaixinWeeklyIndex(html, caixinWeeklyIndexURL)
	if err != nil {
		t.Fatalf("parseCaixinWeeklyIndex failed: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	if issues[0].IssueID != "cw1192" || issues[1].IssueID != "cw1191" {
		t.Fatalf("unexpected sort or IDs: %+v", issues)
	}
}

func TestParseCaixinWeeklyIssueArticleURLs(t *testing.T) {
	html := `
	<html><body>
	<a href="https://www.caixin.com/2026-02-02/102287249.html">Article A</a>
	<a href="/2026-02-02/102287250.html?foo=bar">Article B</a>
	<a href="https://weekly.caixin.com/2026/cw1192/">Issue Home</a>
	<a href="https://u.caixin.com/web/login">Login</a>
	<a href="https://www.caixin.com/video/2026-02-02/102287251.mp4">Not html</a>
	<a href="https://www.caixin.com/2026-02-02/102287250.html">Article B duplicated</a>
	</body></html>
	`

	urls, err := parseCaixinWeeklyIssueArticleURLs(html, "https://weekly.caixin.com/2026/cw1192/")
	if err != nil {
		t.Fatalf("parseCaixinWeeklyIssueArticleURLs failed: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 unique article URLs, got %d, urls=%v", len(urls), urls)
	}
}

func TestChooseCaixinBootstrapIssues(t *testing.T) {
	issues := []caixinWeeklyIssueMeta{
		{IssueID: "cw1192", IssueNumber: 1192},
		{IssueID: "cw1191", IssueNumber: 1191},
	}

	baseline, bootstrap, err := chooseCaixinBootstrapIssues(issues, true)
	if err != nil {
		t.Fatalf("chooseCaixinBootstrapIssues failed: %v", err)
	}
	if baseline.IssueID != "cw1192" {
		t.Fatalf("unexpected baseline: %+v", baseline)
	}
	if bootstrap == nil || bootstrap.IssueID != "cw1191" {
		t.Fatalf("unexpected bootstrap: %+v", bootstrap)
	}
}

func TestBootstrapCaixinWeeklyIssues_DBState(t *testing.T) {
	restore := setupTestDB(t)
	defer restore()

	agent := &Caixin{
		conf: &AgentConf{
			CaixinWeeklyBootstrap: true,
		},
	}
	issues := []caixinWeeklyIssueMeta{
		{
			IssueID:     "cw1192",
			IssueNumber: 1192,
			IssueURL:    "https://weekly.caixin.com/2026/cw1192/",
		},
		{
			IssueID:     "cw1191",
			IssueNumber: 1191,
			IssueURL:    "https://weekly.caixin.com/2026/cw1191/",
		},
	}

	if err := agent.bootstrapCaixinWeeklyIssues(issues); err != nil {
		t.Fatalf("bootstrapCaixinWeeklyIssues failed: %v", err)
	}

	baseline, err := getCaixinWeeklyBaselineIssue()
	if err != nil {
		t.Fatalf("getCaixinWeeklyBaselineIssue failed: %v", err)
	}
	if baseline == nil || baseline.IssueID != "cw1192" {
		t.Fatalf("unexpected baseline issue: %+v", baseline)
	}

	needSyncIssues, err := listCaixinWeeklyIssuesNeedingSync()
	if err != nil {
		t.Fatalf("listCaixinWeeklyIssuesNeedingSync failed: %v", err)
	}
	if len(needSyncIssues) != 1 || needSyncIssues[0].IssueID != "cw1191" {
		t.Fatalf("unexpected issues needing sync: %+v", needSyncIssues)
	}
}

func TestBootstrapCaixinWeeklyIssues_IsIdempotentOnRestart(t *testing.T) {
	restore := setupTestDB(t)
	defer restore()

	agent := &Caixin{
		conf: &AgentConf{
			CaixinWeeklyBootstrap: true,
		},
	}
	issues := []caixinWeeklyIssueMeta{
		{IssueID: "cw1192", IssueNumber: 1192, IssueURL: "https://weekly.caixin.com/2026/cw1192/"},
		{IssueID: "cw1191", IssueNumber: 1191, IssueURL: "https://weekly.caixin.com/2026/cw1191/"},
	}

	if err := agent.bootstrapCaixinWeeklyIssues(issues); err != nil {
		t.Fatalf("first bootstrapCaixinWeeklyIssues failed: %v", err)
	}
	if err := agent.bootstrapCaixinWeeklyIssues(issues); err != nil {
		t.Fatalf("second bootstrapCaixinWeeklyIssues failed: %v", err)
	}

	var count int64
	if err := db.Model(&CaixinWeeklyIssue{}).Count(&count).Error; err != nil {
		t.Fatalf("count issues failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 tracked issues after repeated bootstrap, got %d", count)
	}
}

func setupTestDB(t *testing.T) func() {
	t.Helper()

	orig := db
	dbPath := filepath.Join(t.TempDir(), "readform_test.db")
	testDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
	})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := testDB.AutoMigrate(&Article{}, &CaixinWeeklyIssue{}, &CaixinWeeklyIssueArticle{}); err != nil {
		t.Fatalf("automigrate failed: %v", err)
	}
	db = testDB

	return func() {
		db = orig
	}
}
