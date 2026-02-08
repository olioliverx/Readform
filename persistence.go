package main

import (
	"errors"
	"fmt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Article struct {
	URL             string `gorm:"type:varchar(2048);primaryKey"`
	Agent           string `gorm:"type:varchar(128)"`
	SavedToReadwise bool   `gorm:"type:boolean"`
	SaveTime        string `gorm:"type:varchar(128)"`
	ReadwiseResp    string `gorm:"type:varchar(1024)"`
	// Content         string `gorm:"type:varchar"`
	ActualURL   string
	PublishTime time.Time `gorm:"type:datetime"`
	CreateTime  time.Time `gorm:"type:datetime;autoCreateTime"`
	UpdateTime  time.Time `gorm:"type:datetime;autoUpdateTime"`
}

const (
	CaixinWeeklyIssueStatusPending    = "pending"
	CaixinWeeklyIssueStatusInProgress = "in_progress"
	CaixinWeeklyIssueStatusComplete   = "complete"
	CaixinWeeklyIssueStatusIncomplete = "incomplete"
)

type CaixinWeeklyIssue struct {
	IssueID              string    `gorm:"type:varchar(128);primaryKey"`
	IssueURL             string    `gorm:"type:varchar(2048)"`
	IssueTitle           string    `gorm:"type:varchar(512)"`
	IssueNumber          int       `gorm:"type:int;index"`
	PublishDate          time.Time `gorm:"type:datetime;index"`
	IsBaseline           bool      `gorm:"type:boolean;index"`
	IsBootstrap          bool      `gorm:"type:boolean"`
	ExpectedArticleCount int       `gorm:"type:int"`
	SavedArticleCount    int       `gorm:"type:int"`
	Status               string    `gorm:"type:varchar(64);index"`
	LastError            string    `gorm:"type:varchar(2048)"`
	CreateTime           time.Time `gorm:"type:datetime;autoCreateTime"`
	UpdateTime           time.Time `gorm:"type:datetime;autoUpdateTime"`
}

type CaixinWeeklyIssueArticle struct {
	IssueID    string    `gorm:"type:varchar(128);primaryKey"`
	URL        string    `gorm:"type:varchar(2048);primaryKey"`
	CreateTime time.Time `gorm:"type:datetime;autoCreateTime"`
	UpdateTime time.Time `gorm:"type:datetime;autoUpdateTime"`
}

var db *gorm.DB

func initDB() {
	var err error
	db, err = gorm.Open(sqlite.Open("data/readform.db"), &gorm.Config{NamingStrategy: schema.NamingStrategy{
		SingularTable: true,
	}})
	if err != nil {
		panic("failed to connect database")
	}
	err = db.AutoMigrate(&Article{}, &CaixinWeeklyIssue{}, &CaixinWeeklyIssueArticle{})
	if err != nil {
		panic(err)
	}
}

func addArticle(url string, agent string, actualURL string) error {
	var articles []*Article
	err := db.Find(&articles, "url = ?", url).Error
	if err != nil {
		return err
	}
	if len(articles) == 0 {
		// Article does not exist, create a new one
		article := Article{
			URL:       url,
			ActualURL: actualURL,
			Agent:     agent,
		}
		return db.Create(&article).Error
	} else {
		// Article exists, update it
		return db.Model(&articles[0]).Updates(Article{Agent: agent}).Error
	}
}

func markURLAsSaved(url string, agent string, resp string) error {
	var article Article
	if err := db.First(&article, "url = ?", url).Error; err != nil {
		// Article does not exist, create a new one
		article = Article{
			URL:             url,
			Agent:           agent,
			SavedToReadwise: true,
			SaveTime:        time.Now().Format("2006-01-02 15:04:05"),
			ReadwiseResp:    resp,
		}
		return db.Create(&article).Error
	} else {
		// Article exists, update it
		return db.Model(&article).Updates(Article{
			SavedToReadwise: true,
			SaveTime:        time.Now().Format("2006-01-02 15:04:05"),
			Agent:           agent,
			ReadwiseResp:    resp,
		}).Error
	}
}

// findArticle finds article from database. Legacy versions of Readform does not have ActualURL field,
// so hasActualURL=true can filter out items created by legacy version.
func findArticle(urlList []string, onlySaved bool, onlyNotSaved bool, hasActualURL bool) ([]Article, error) {
	var articles []Article
	tx := db
	if urlList != nil {
		tx = tx.Where("url IN (?)", urlList)
	}
	if onlySaved {
		tx = tx.Where("saved_to_readwise = ?", true)
	}
	if onlyNotSaved {
		tx = tx.Where("saved_to_readwise = ?", false)
	}
	if hasActualURL {
		tx = tx.Where("actual_url != ''")
	}

	if err := tx.Find(&articles).Error; err != nil {
		return nil, err
	}

	return articles, nil
}

// filterOldURLs filter out saved URLs, returning unsaved URLs.
func filterOldURLs(urls []string) ([]string, error) {
	articles, err := findArticle(urls, true, false, false)
	if err != nil {
		return nil, fmt.Errorf("findArticle failed: %w", err)
	}
	existURLs := make(map[string]struct{}, len(articles))
	for _, a := range articles {
		existURLs[a.URL] = struct{}{}
	}

	var unsavedURLs []string
	for _, url := range urls {
		if _, exist := existURLs[url]; !exist {
			unsavedURLs = append(unsavedURLs, url)
		}
	}
	unsavedURLs = UniqStringSlice(unsavedURLs)
	return unsavedURLs, nil
}

func getCaixinWeeklyIssue(issueID string) (*CaixinWeeklyIssue, error) {
	var issue CaixinWeeklyIssue
	err := db.First(&issue, "issue_id = ?", issueID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &issue, nil
}

func upsertCaixinWeeklyIssue(issue *CaixinWeeklyIssue) error {
	if issue.Status == "" {
		issue.Status = CaixinWeeklyIssueStatusPending
	}
	return db.Save(issue).Error
}

func getCaixinWeeklyBaselineIssue() (*CaixinWeeklyIssue, error) {
	var issue CaixinWeeklyIssue
	err := db.Where("is_baseline = ?", true).Order("issue_number desc").First(&issue).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &issue, nil
}

func setCaixinWeeklyBaseline(issueID string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&CaixinWeeklyIssue{}).Where("is_baseline = ?", true).Update("is_baseline", false).Error; err != nil {
			return err
		}
		return tx.Model(&CaixinWeeklyIssue{}).Where("issue_id = ?", issueID).Update("is_baseline", true).Error
	})
}

func upsertCaixinWeeklyIssueArticles(issueID string, urls []string) error {
	uniqueURLs := UniqStringSlice(urls)
	return db.Transaction(func(tx *gorm.DB) error {
		if len(uniqueURLs) == 0 {
			if err := tx.Where("issue_id = ?", issueID).Delete(&CaixinWeeklyIssueArticle{}).Error; err != nil {
				return err
			}
			return tx.Model(&CaixinWeeklyIssue{}).Where("issue_id = ?", issueID).Update("expected_article_count", 0).Error
		}

		if err := tx.Where("issue_id = ? AND url NOT IN (?)", issueID, uniqueURLs).Delete(&CaixinWeeklyIssueArticle{}).Error; err != nil {
			return err
		}

		for _, articleURL := range uniqueURLs {
			row := CaixinWeeklyIssueArticle{
				IssueID: issueID,
				URL:     articleURL,
			}
			if err := tx.FirstOrCreate(&row, CaixinWeeklyIssueArticle{IssueID: issueID, URL: articleURL}).Error; err != nil {
				return err
			}
		}

		return tx.Model(&CaixinWeeklyIssue{}).Where("issue_id = ?", issueID).Update("expected_article_count", len(uniqueURLs)).Error
	})
}

func listCaixinWeeklyIssuesNeedingSync() ([]CaixinWeeklyIssue, error) {
	var issues []CaixinWeeklyIssue
	err := db.Where("is_baseline = ?", false).
		Where("status <> ?", CaixinWeeklyIssueStatusComplete).
		Order("issue_number asc").
		Find(&issues).Error
	if err != nil {
		return nil, err
	}
	return issues, nil
}

func countCaixinWeeklyIssueProgress(issueID string) (saved int, total int, err error) {
	var totalCount int64
	if err := db.Model(&CaixinWeeklyIssueArticle{}).Where("issue_id = ?", issueID).Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	var savedCount int64
	err = db.Table("caixin_weekly_issue_article cwa").
		Joins("JOIN article a ON a.url = cwa.url AND a.saved_to_readwise = ?", true).
		Where("cwa.issue_id = ?", issueID).
		Distinct("cwa.url").
		Count(&savedCount).Error
	if err != nil {
		return 0, 0, err
	}

	return int(savedCount), int(totalCount), nil
}

func listUnsavedCaixinWeeklyIssueArticles(issueID string) ([]string, error) {
	var urls []string
	err := db.Table("caixin_weekly_issue_article cwa").
		Select("cwa.url").
		Joins("LEFT JOIN article a ON a.url = cwa.url").
		Where("cwa.issue_id = ?", issueID).
		Where("a.url IS NULL OR a.saved_to_readwise = ?", false).
		Scan(&urls).Error
	if err != nil {
		return nil, err
	}
	return urls, nil
}

func updateCaixinWeeklyIssueStatus(issueID string, status string, lastErr string) error {
	saved, total, err := countCaixinWeeklyIssueProgress(issueID)
	if err != nil {
		return err
	}

	updates := map[string]interface{}{
		"status":                status,
		"saved_article_count":   saved,
		"expected_article_count": total,
	}
	if lastErr != "" {
		updates["last_error"] = lastErr
	} else {
		updates["last_error"] = ""
	}
	return db.Model(&CaixinWeeklyIssue{}).Where("issue_id = ?", issueID).Updates(updates).Error
}

func urlToLocalFilePath(url string) string {
	fileName := strings.ReplaceAll(url, "/", "_")
	fileName = strings.ReplaceAll(fileName, ":", "")
	filePath := "data/html/" + fileName + ".html"
	return filePath
}

// saveHTMLToLocalFile saves URL content to local file.
func saveHTMLToLocalFile(url, htmlContent string) error {
	filePath := urlToLocalFilePath(url)

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory failed: %w", err)
	}

	err := os.WriteFile(filePath, []byte(htmlContent), 0644)
	if err != nil {
		return fmt.Errorf("WriteFile failed: %w", err)
	}
	return nil
}

// readLocalHTMLFile gets URL content from local file.
func readLocalHTMLFile(url string) (string, error) {
	filePath := urlToLocalFilePath(url)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
