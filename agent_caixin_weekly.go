package main

import (
	"context"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	caixinWeeklyIndexURL = "https://weekly.caixin.com/"
	caixinLoginURL       = "https://u.caixin.com/web/login"
	caixinWeeklyRSSURL   = "https://rsshub.app/caixin/weekly"
)

var (
	caixinIssuePathPattern = regexp.MustCompile(`/\d{4}/(cw(\d+))/?`)
	caixinDatePatternDash  = regexp.MustCompile(`(\d{4})-(\d{1,2})-(\d{1,2})`)
	caixinDatePatternSlash = regexp.MustCompile(`(\d{4})/(\d{1,2})/(\d{1,2})`)
	caixinDatePatternCN    = regexp.MustCompile(`(\d{4})年(\d{1,2})月(\d{1,2})日`)
	fileSafePattern        = regexp.MustCompile(`[^a-zA-Z0-9]+`)
)

type caixinWeeklyIssueMeta struct {
	IssueID     string
	IssueNumber int
	IssueURL    string
	IssueTitle  string
	PublishDate time.Time
}

func (a *Caixin) DiscoverArticleURLs(agent *WebsiteAgent, isFirstRun bool) (CustomDiscoveryResult, error) {
	if a.getContentMode() != CaixinContentModeWeeklyOnly {
		urls, err := agent.refreshRSS()
		if err != nil {
			return CustomDiscoveryResult{}, err
		}
		return CustomDiscoveryResult{URLs: urls}, nil
	}

	if isFirstRun {
		if err := a.preflightLogin(agent); err != nil {
			return CustomDiscoveryResult{}, err
		}
	}

	urls, err := a.discoverCaixinWeeklyArticleURLs(agent)
	if err != nil {
		return CustomDiscoveryResult{}, err
	}

	return CustomDiscoveryResult{
		URLs:                  urls,
		ForceProcessFirstFetch: true,
	}, nil
}

func (a *Caixin) getContentMode() string {
	if a.conf == nil {
		return CaixinContentModeLatest
	}
	mode := strings.TrimSpace(strings.ToLower(a.conf.CaixinContentMode))
	if mode == "" {
		return CaixinContentModeLatest
	}
	if mode != CaixinContentModeLatest && mode != CaixinContentModeWeeklyOnly {
		logger.Warnf("[caixin] unknown content mode %s, fallback to latest", mode)
		return CaixinContentModeLatest
	}
	return mode
}

func (a *Caixin) shouldBootstrapLastIssue() bool {
	if a.conf == nil {
		return true
	}
	return a.conf.CaixinWeeklyBootstrap
}

func (a *Caixin) preflightLogin(agent *WebsiteAgent) error {
	if strings.TrimSpace(a.conf.Username) == "" {
		return fmt.Errorf("[caixin] preflight failed: username is empty")
	}
	if strings.TrimSpace(a.conf.Password) == "" {
		return fmt.Errorf("[caixin] preflight failed: password is empty")
	}

	ctx, cancel := context.WithTimeout(agent.ctx, 90*time.Second)
	defer cancel()

	tabCtx, tabCancel := chromedp.NewContext(ctx)
	defer tabCancel()

	runStep := func(step string, actions ...chromedp.Action) error {
		if err := chromedp.Run(tabCtx, actions...); err != nil {
			return a.wrapStepErrWithScreenshot(tabCtx, step, err)
		}
		return nil
	}

	if err := runStep("login_preflight_navigate", chromedp.Navigate(caixinLoginURL), browser.WaitUntilDocumentReady()); err != nil {
		return err
	}

	// Caixin login page defaults to QR code; click "其他方式登录" to reveal mobile/password form
	if err := runStep("login_preflight_switch_to_password",
		chromedp.WaitVisible(`//*[contains(text(), '其他方式登录')]`),
		chromedp.Click(`//*[contains(text(), '其他方式登录')]`),
		chromedp.Sleep(2*time.Second),
	); err != nil {
		return err
	}

	// Click consent/agreement checkbox
	if err := runStep("login_preflight_consent",
		chromedp.Evaluate(`
			(function(){
				var cb = document.querySelector(".cx-login-argree input[type='checkbox']");
				if (cb) { cb.click(); return "checkbox"; }
				var span = document.querySelector(".cx-login-argree label span span");
				if (span) { span.click(); return "span"; }
				var label = document.querySelector(".cx-login-argree label");
				if (label) { label.click(); return "label"; }
				return "not_found";
			})()
		`, nil),
		chromedp.Sleep(1*time.Second),
	); err != nil {
		logger.Warnf("[caixin] preflight consent click failed (non-fatal): %v", err)
	}

	// Try multiple selectors for the mobile input field (Caixin may change these)
	mobileSelectors := []string{
		`input[name='mobile']`,
		`input[name='phone']`,
		`input[type='tel']`,
		`input[placeholder*='手机']`,
		`input[placeholder*='号码']`,
	}
	mobileFound := false
	for _, sel := range mobileSelectors {
		var nodes []*cdp.Node
		if err := chromedp.Run(tabCtx, chromedp.Nodes(sel, &nodes, chromedp.AtLeast(0))); err == nil && len(nodes) > 0 {
			logger.Infof("[caixin] preflight: mobile input found with selector: %s", sel)
			mobileFound = true
			break
		}
	}
	if !mobileFound {
		// Dump page HTML for debugging
		var pageHTML string
		_ = chromedp.Run(tabCtx, chromedp.OuterHTML("html", &pageHTML))
		if len(pageHTML) > 2000 {
			pageHTML = pageHTML[:2000]
		}
		logger.Errorf("[caixin] preflight: no mobile input found. Page HTML (truncated): %s", pageHTML)
		return a.wrapStepErrWithScreenshot(tabCtx, "login_preflight_no_mobile_input", fmt.Errorf("no mobile input selector matched"))
	}

	// Verify password input and login button exist
	otherSelectors := []string{
		`input[name='password'],input[type='password']`,
		`button.login-btn,button[type='submit']`,
	}
	for _, sel := range otherSelectors {
		var nodes []*cdp.Node
		step := "login_preflight_selector_" + sanitizeForFileName(sel)
		if err := runStep(step, chromedp.Nodes(sel, &nodes, chromedp.AtLeast(1))); err != nil {
			return err
		}
	}

	return nil
}

func (a *Caixin) discoverCaixinWeeklyArticleURLs(agent *WebsiteAgent) ([]string, error) {
	weeklyIssues, err := a.fetchCaixinWeeklyIssues()
	if err != nil {
		return nil, err
	}
	if len(weeklyIssues) == 0 {
		return nil, fmt.Errorf("[caixin] no weekly issues found")
	}

	weeklyIssuesByID := make(map[string]caixinWeeklyIssueMeta, len(weeklyIssues))
	for _, issue := range weeklyIssues {
		weeklyIssuesByID[issue.IssueID] = issue
	}

	baselineIssue, err := getCaixinWeeklyBaselineIssue()
	if err != nil {
		return nil, err
	}
	if baselineIssue == nil {
		if err := a.bootstrapCaixinWeeklyIssues(weeklyIssues); err != nil {
			return nil, err
		}
		baselineIssue, err = getCaixinWeeklyBaselineIssue()
		if err != nil {
			return nil, err
		}
		if baselineIssue == nil {
			return nil, fmt.Errorf("[caixin] baseline issue is empty after bootstrap")
		}
	}

	for _, issue := range weeklyIssues {
		if issue.IssueNumber <= baselineIssue.IssueNumber {
			continue
		}
		existing, err := getCaixinWeeklyIssue(issue.IssueID)
		if err != nil {
			return nil, err
		}

		dbIssue := a.issueMetaToDB(issue)
		dbIssue.IsBaseline = false
		dbIssue.IsBootstrap = false
		dbIssue.Status = CaixinWeeklyIssueStatusPending
		if existing != nil {
			dbIssue.IsBaseline = existing.IsBaseline
			dbIssue.IsBootstrap = existing.IsBootstrap
			dbIssue.Status = existing.Status
		}
		if err := upsertCaixinWeeklyIssue(dbIssue); err != nil {
			return nil, err
		}
	}

	issuePendingURLs, err := a.collectUnsavedArticlesFromTrackedIssues(weeklyIssuesByID)
	if err != nil {
		return nil, err
	}

	rssPendingURLs, err := a.discoverCaixinWeeklyRSSURLs(agent)
	if err != nil {
		logger.Warnf("[caixin] weekly RSS discovery failed, continue with crawler-only: %v", err)
	}

	return UniqStringSlice(append(issuePendingURLs, rssPendingURLs...)), nil
}

func (a *Caixin) bootstrapCaixinWeeklyIssues(issues []caixinWeeklyIssueMeta) error {
	baseline, bootstrap, err := chooseCaixinBootstrapIssues(issues, a.shouldBootstrapLastIssue())
	if err != nil {
		return err
	}

	baselineIssue := a.issueMetaToDB(baseline)
	baselineIssue.IsBaseline = true
	baselineIssue.IsBootstrap = false
	baselineIssue.Status = CaixinWeeklyIssueStatusComplete
	if err := upsertCaixinWeeklyIssue(baselineIssue); err != nil {
		return err
	}
	if err := setCaixinWeeklyBaseline(baseline.IssueID); err != nil {
		return err
	}
	logger.Infof("[caixin] baseline issue set to %s (%s)", baseline.IssueID, baseline.IssueURL)

	if bootstrap == nil {
		logger.Infof("[caixin] bootstrap issue disabled or unavailable")
		return nil
	}

	bootstrapIssue := a.issueMetaToDB(*bootstrap)
	bootstrapIssue.IsBaseline = false
	bootstrapIssue.IsBootstrap = true
	bootstrapIssue.Status = CaixinWeeklyIssueStatusPending
	if err := upsertCaixinWeeklyIssue(bootstrapIssue); err != nil {
		return err
	}
	logger.Infof("[caixin] bootstrap issue queued: %s (%s)", bootstrap.IssueID, bootstrap.IssueURL)
	return nil
}

func chooseCaixinBootstrapIssues(issues []caixinWeeklyIssueMeta, enableBootstrap bool) (baseline caixinWeeklyIssueMeta, bootstrap *caixinWeeklyIssueMeta, err error) {
	if len(issues) == 0 {
		return caixinWeeklyIssueMeta{}, nil, fmt.Errorf("issues is empty")
	}

	baseline = issues[0]
	if !enableBootstrap || len(issues) < 2 {
		return baseline, nil, nil
	}

	issue := issues[1]
	bootstrap = &issue
	return baseline, bootstrap, nil
}

func (a *Caixin) issueMetaToDB(issue caixinWeeklyIssueMeta) *CaixinWeeklyIssue {
	return &CaixinWeeklyIssue{
		IssueID:     issue.IssueID,
		IssueURL:    issue.IssueURL,
		IssueTitle:  issue.IssueTitle,
		IssueNumber: issue.IssueNumber,
		PublishDate: issue.PublishDate,
	}
}

func (a *Caixin) collectUnsavedArticlesFromTrackedIssues(issueMetaMap map[string]caixinWeeklyIssueMeta) ([]string, error) {
	issues, err := listCaixinWeeklyIssuesNeedingSync()
	if err != nil {
		return nil, err
	}

	var pendingURLs []string
	for _, issue := range issues {
		if issue.IssueURL == "" {
			if meta, ok := issueMetaMap[issue.IssueID]; ok {
				issue.IssueURL = meta.IssueURL
			}
		}
		if issue.IssueURL == "" {
			if err := updateCaixinWeeklyIssueStatus(issue.IssueID, CaixinWeeklyIssueStatusIncomplete, "empty issue URL"); err != nil {
				return nil, err
			}
			continue
		}

		issueArticleURLs, err := a.fetchCaixinWeeklyIssueArticleURLs(issue.IssueURL)
		if err != nil {
			if updateErr := updateCaixinWeeklyIssueStatus(issue.IssueID, CaixinWeeklyIssueStatusIncomplete, err.Error()); updateErr != nil {
				return nil, updateErr
			}
			continue
		}

		if err := upsertCaixinWeeklyIssueArticles(issue.IssueID, issueArticleURLs); err != nil {
			return nil, err
		}

		unsavedURLs, err := listUnsavedCaixinWeeklyIssueArticles(issue.IssueID)
		if err != nil {
			return nil, err
		}

		if len(issueArticleURLs) == 0 {
			if err := updateCaixinWeeklyIssueStatus(issue.IssueID, CaixinWeeklyIssueStatusIncomplete, "issue has no article URLs"); err != nil {
				return nil, err
			}
			continue
		}

		if len(unsavedURLs) == 0 {
			if err := updateCaixinWeeklyIssueStatus(issue.IssueID, CaixinWeeklyIssueStatusComplete, ""); err != nil {
				return nil, err
			}
			continue
		}

		if err := updateCaixinWeeklyIssueStatus(issue.IssueID, CaixinWeeklyIssueStatusInProgress, ""); err != nil {
			return nil, err
		}
		pendingURLs = append(pendingURLs, unsavedURLs...)
	}

	return UniqStringSlice(pendingURLs), nil
}

func (a *Caixin) fetchCaixinWeeklyIssues() ([]caixinWeeklyIssueMeta, error) {
	htmlContent, err := fetchWebpageContent(caixinWeeklyIndexURL)
	if err != nil {
		return nil, err
	}
	issues, err := parseCaixinWeeklyIndex(htmlContent, caixinWeeklyIndexURL)
	if err != nil {
		return nil, err
	}
	return issues, nil
}

func (a *Caixin) fetchCaixinWeeklyIssueArticleURLs(issueURL string) ([]string, error) {
	htmlContent, err := fetchWebpageContent(issueURL)
	if err != nil {
		return nil, err
	}
	return parseCaixinWeeklyIssueArticleURLs(htmlContent, issueURL)
}

func (a *Caixin) discoverCaixinWeeklyRSSURLs(agent *WebsiteAgent) ([]string, error) {
	feedItems, err := ParseRssFeed(caixinWeeklyRSSURL)
	if err != nil {
		return nil, err
	}

	var urls []string
	for _, item := range feedItems {
		if agent != nil && agent.containsBlockedKeyword(item.Title) {
			continue
		}
		urls = append(urls, item.Link)
	}
	urls, err = filterOldURLs(UniqStringSlice(urls))
	if err != nil {
		return nil, err
	}
	return urls, nil
}

func fetchWebpageContent(targetURL string) (string, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Readform Weekly Crawler)")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("request %s failed with status %d", targetURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func parseCaixinWeeklyIndex(htmlContent string, baseURL string) ([]caixinWeeklyIssueMeta, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	issuesByID := make(map[string]caixinWeeklyIssueMeta)
	doc.Find("a[href]").Each(func(_ int, selection *goquery.Selection) {
		href, ok := selection.Attr("href")
		if !ok {
			return
		}
		issueURL, ok := resolveRelativeURL(baseURL, href)
		if !ok {
			return
		}
		issueID, issueNumber, ok := extractIssueIDAndNumber(issueURL)
		if !ok {
			return
		}

		title := strings.TrimSpace(selection.Text())
		if title == "" {
			title = issueID
		}

		parentText := strings.TrimSpace(selection.Parent().Text())
		publishDate := parseDateFromText(parentText)
		if publishDate.IsZero() {
			publishDate = parseDateFromText(strings.TrimSpace(selection.Text()))
		}

		issue := caixinWeeklyIssueMeta{
			IssueID:     issueID,
			IssueNumber: issueNumber,
			IssueURL:    issueURL,
			IssueTitle:  title,
			PublishDate: publishDate,
		}

		existing, exists := issuesByID[issueID]
		if !exists {
			issuesByID[issueID] = issue
			return
		}
		if existing.IssueTitle == "" || existing.IssueTitle == existing.IssueID {
			existing.IssueTitle = issue.IssueTitle
		}
		if existing.PublishDate.IsZero() && !issue.PublishDate.IsZero() {
			existing.PublishDate = issue.PublishDate
		}
		issuesByID[issueID] = existing
	})

	issues := make([]caixinWeeklyIssueMeta, 0, len(issuesByID))
	for _, issue := range issuesByID {
		issues = append(issues, issue)
	}

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].IssueNumber != issues[j].IssueNumber {
			return issues[i].IssueNumber > issues[j].IssueNumber
		}
		return issues[i].PublishDate.After(issues[j].PublishDate)
	})

	return issues, nil
}

func parseCaixinWeeklyIssueArticleURLs(htmlContent string, baseURL string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	var urls []string
	doc.Find("a[href]").Each(func(_ int, selection *goquery.Selection) {
		href, ok := selection.Attr("href")
		if !ok {
			return
		}

		articleURL, ok := resolveRelativeURL(baseURL, href)
		if !ok {
			return
		}
		if !isLikelyCaixinArticleURL(articleURL) {
			return
		}
		urls = append(urls, articleURL)
	})

	return UniqStringSlice(urls), nil
}

func resolveRelativeURL(baseURL string, raw string) (string, bool) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", false
	}
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	resolved := base.ResolveReference(ref)
	resolved.RawQuery = ""
	resolved.Fragment = ""
	return resolved.String(), true
}

func extractIssueIDAndNumber(issueURL string) (issueID string, issueNumber int, ok bool) {
	matches := caixinIssuePathPattern.FindStringSubmatch(issueURL)
	if len(matches) < 3 {
		return "", 0, false
	}
	issueID = strings.ToLower(matches[1])
	number, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", 0, false
	}
	return issueID, number, true
}

func parseDateFromText(text string) time.Time {
	text = strings.TrimSpace(text)
	for _, pattern := range []*regexp.Regexp{caixinDatePatternDash, caixinDatePatternSlash, caixinDatePatternCN} {
		matches := pattern.FindStringSubmatch(text)
		if len(matches) < 4 {
			continue
		}
		year, yErr := strconv.Atoi(matches[1])
		month, mErr := strconv.Atoi(matches[2])
		day, dErr := strconv.Atoi(matches[3])
		if yErr != nil || mErr != nil || dErr != nil {
			continue
		}
		return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)
	}
	return time.Time{}
}

func isLikelyCaixinArticleURL(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsedURL.Hostname())
	if !strings.HasSuffix(host, "caixin.com") {
		return false
	}
	if host == "weekly.caixin.com" || host == "u.caixin.com" {
		return false
	}

	path := strings.ToLower(parsedURL.Path)
	if !strings.HasSuffix(path, ".html") {
		return false
	}
	return true
}

func sanitizeForFileName(input string) string {
	cleaned := fileSafePattern.ReplaceAllString(strings.ToLower(input), "_")
	cleaned = strings.Trim(cleaned, "_")
	if cleaned == "" {
		return "unknown"
	}
	return cleaned
}
