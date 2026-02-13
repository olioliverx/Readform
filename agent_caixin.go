package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Caixin struct {
	conf *AgentConf

	paywallLocator string
}

func (a *Caixin) Name() string {
	return "caixin"
}

func (a *Caixin) DisplayName() string {
	return "Caixin"
}

func (a *Caixin) ConfOptions() []ConfMeta {
	return []ConfMeta{
		{
			ConfigName:        "Caixin Username",
			ConfigDescription: "Your username for Caixin.",
			ConfigKey:         AgentConfUsername,
			Type:              FieldTypeString,
			Required:          true,
		},
		{
			ConfigName:        "Caixin Password",
			ConfigDescription: "Your password for Caixin.",
			ConfigKey:         AgentConfPassword,
			Type:              FieldTypeString,
			Required:          true,
		},
		{
			ConfigName:        "Keyword Blocklist",
			ConfigDescription: "Keywords you want to filter out. Split by comma(,).",
			ConfigKey:         AgentConfKeyBlocklist,
			Type:              FieldTypeStringList,
			Required:          false,
		},
		{
			ConfigName:        "Custom RSS feed link",
			ConfigDescription: "Default feed link is https://rsshub.app/caixin/latest. You can replace it with your own wanted feed link. Multiple links should split by comma(,).",
			ConfigKey:         AgentConfKeyRSSLinks,
			Type:              FieldTypeStringList,
			Required:          false,
		},
		{
			ConfigName:        "Caixin content mode",
			ConfigDescription: "latest keeps legacy RSS-only behavior. weekly_only ingests previous complete weekly issue once, then syncs newer weekly issues.",
			ConfigKey:         AgentConfCaixinContentMode,
			Type:              FieldTypeRadio,
			SelectOptions: []SelectOption{
				{Value: CaixinContentModeLatest, DisplayName: "Latest (legacy RSS)"},
				{Value: CaixinContentModeWeeklyOnly, DisplayName: "Weekly only"},
			},
			DefaultValue: CaixinContentModeLatest,
			Required:     true,
		},
		{
			ConfigName:        "Bootstrap previous weekly issue once",
			ConfigDescription: "When enabled in weekly_only mode, first startup ingests the previous complete weekly issue once as a verification batch.",
			ConfigKey:         AgentConfCaixinWeeklyBootstrap,
			Type:              FieldTypeBool,
			DefaultValue:      TrueLiteral,
			Required:          true,
		},
		{
			ConfigName:        "Include data-pass(数据通) articles",
			ConfigDescription: "This requires higher level of subscription. Default is false.",
			ConfigKey:         AgentConfIncludePremiumArticles,
			Type:              FieldTypeBool,
			Required:          false,
		},
	}
}

func (a *Caixin) BaseDomains() []string {
	return []string{"caixin.com"}
}

func (a *Caixin) TestPage() string {
	return "https://www.caixin.com/"
}

func (a *Caixin) URLPrefixBlockList() []string {
	prefixList := []string{
		"https://photos.caixin.com",
	}
	if !a.conf.IncludePremiumArticles {
		prefixList = append(prefixList, "https://database.caixin.com")
	}
	return prefixList
}

func (a *Caixin) RSSLinks() []string {
	return []string{"https://rsshub.app/caixin/latest"}
}

func (a *Caixin) RequireScrolling() bool {
	return true
}

func (a *Caixin) Init(conf *AgentConf) error {
	a.paywallLocator = "#chargeWallContent"
	if conf == nil {
		conf = &AgentConf{
			CaixinContentMode:     CaixinContentModeLatest,
			CaixinWeeklyBootstrap: true,
		}
	}
	a.conf = conf
	if a.conf.CaixinContentMode == "" {
		a.conf.CaixinContentMode = CaixinContentModeLatest
	}
	return nil
}

func (a *Caixin) CleanURL(url string) (string, error) {
	return url, nil
}

func (a *Caixin) CheckFinishLoading(ctx context.Context) error {
	logger.Infof("waiting for article body to load...")
	_, err := browser.GetWebElementWithWait(ctx, "#the_content")
	if err != nil {
		return err
	}

	err = browser.WaitUntilInvisible(ctx, "#loadinWall")
	if err != nil {
		return fmt.Errorf("waitUntilInvisible failed: %w", err)
	}

	// Check if video exist
	// Readwise Reader seems cannot handle this type of video yet. But we still make sure it to exist
	// in HTML for future usage.
	var nodes []*cdp.Node
	err = chromedp.Run(ctx, chromedp.Nodes("div.content_video", &nodes, chromedp.AtLeast(0)))
	if err != nil {
		return err
	} else if len(nodes) > 0 {
		logger.Infof("video found, wait for it to load...")
		_, err = browser.GetWebElementWithWait(ctx, "div.cx-audio-rep")
		if err != nil {
			return err
		}
	}
	logger.Infof("body loading finished")

	return nil
}

func (a *Caixin) IsArticlePaywalled(ctx context.Context) (bool, error) {
	return a.isPaywalled(ctx)
}

func (a *Caixin) EnsureLoggedIn(ctx context.Context) error {
	isPaywalled, err := a.isPaywalled(ctx)
	if err != nil {
		return fmt.Errorf("isPaywalled failed: %w", err)
	}
	if isPaywalled {
		logger.Infof("is paywalled content and not logged-in")

		err = chromedp.Run(ctx, chromedp.Tasks{
			chromedp.Navigate(caixinLoginURL),
			browser.WaitUntilDocumentReady(),
		})
		if err != nil {
			return fmt.Errorf("failed to go to login page: %w", err)
		}

		currentURL, err := browser.GetCurrentURL(ctx)
		if err != nil {
			return fmt.Errorf("GetCurrentURL failed: %w", err)
		}
		if currentURL == "https://u.caixin.com/web/workbench" {
			// already login
			return fmt.Errorf("already logged in but still paywalled. Check if your subscription is valid")
		}

		err = a.login(ctx)
		if err != nil {
			return fmt.Errorf("failed to login: %w", err)
		}

		return nil
	} else {
		logger.Infof("is not paywalled content or already logged in")
		return nil
	}
}

func (a *Caixin) isPaywalled(ctx context.Context) (bool, error) {
	var nodes []*cdp.Node
	err := chromedp.Run(ctx, chromedp.Nodes(a.paywallLocator, &nodes, chromedp.AtLeast(0)))
	if err != nil {
		return false, err
	} else if len(nodes) == 0 {
		return false, nil
	} else {
		// check if visible
		var nodes []*cdp.Node
		var isHidden bool
		err := chromedp.Run(ctx,
			chromedp.Nodes(a.paywallLocator, &nodes, chromedp.AtLeast(1)),
			browser.IsElementHidden(a.paywallLocator, &isHidden),
		)
		if err != nil {
			return false, fmt.Errorf("check isHidden failed: %w", err)
		}
		if isHidden {
			return false, nil
		}

		var bodyText string
		err = chromedp.Run(ctx, chromedp.OuterHTML("html", &bodyText))
		if err != nil {
			return false, fmt.Errorf("get document body failed: %w", err)
		}
		if strings.Contains(bodyText, "请升级后阅读") {
			return true, fmt.Errorf("当前用户会员等级不足，需要升级后阅读")
		}
		return true, nil
	}
}

func (a *Caixin) login(ctx context.Context) error {
	logger.Infof("logging in...")
	username := a.conf.Username
	if username == "" {
		return errors.New("username is empty, cannot proceed")
	}
	password := a.conf.Password
	if password == "" {
		return errors.New("password is empty, cannot proceed")
	}

	runStep := func(step string, actions ...chromedp.Action) error {
		if err := chromedp.Run(ctx, actions...); err != nil {
			return a.wrapStepErrWithScreenshot(ctx, step, err)
		}
		return nil
	}

	// Wait for Vue SPA to render
	logger.Infof("next step: waiting for SPA to render")
	if err := runStep("wait_render", chromedp.Sleep(3*time.Second)); err != nil {
		return err
	}

	// Try multiple selectors for the mobile input field
	mobileSelectors := []string{
		`input[name='mobile']`,
		`input[placeholder="请输入手机号"]`,
		`input[name='phone']`,
		`input[type='tel']`,
		`input[placeholder*='手机']`,
		`input[placeholder*='号码']`,
	}

	// Check if the mobile/password form is already visible
	mobileSel := ""
	for _, sel := range mobileSelectors {
		var nodes []*cdp.Node
		if err := chromedp.Run(ctx, chromedp.Nodes(sel, &nodes, chromedp.AtLeast(0))); err == nil && len(nodes) > 0 {
			mobileSel = sel
			logger.Infof("mobile input already visible with selector: %s", sel)
			break
		}
	}

	// If mobile input not found, toggle from QR code view to account/password form
	if mobileSel == "" {
		logger.Infof("next step: switching to password login")
		var switchResult string
		if err := runStep("switch_to_password_login",
			chromedp.Evaluate(`
				(function(){
					// Strategy 1: click the ElementUI "手机号登录" tab directly
					var mobileTab = document.getElementById('tab-mobile');
					if (mobileTab) { mobileTab.click(); return "tab-mobile"; }

					// Strategy 2: find tab by text content
					var tabs = document.querySelectorAll('.el-tabs__item');
					for (var i = 0; i < tabs.length; i++) {
						var txt = tabs[i].textContent.trim();
						if (txt === '手机号登录' || txt === '账号登录' || txt === '密码登录') {
							tabs[i].click();
							return "tab_text:" + txt;
						}
					}

					// Strategy 3: click the QR-to-account switcher icon (top-right of login card)
					var switcher = document.querySelector('.cx-icon-switch') ||
						document.querySelector('.cx-login-switch') ||
						document.querySelector('.login-switch') ||
						document.querySelector('.qr-switch');
					if (switcher) { switcher.click(); return "switcher_icon"; }

					// Strategy 4: look for clickable icon in the top area of the login container
					var container = document.querySelector('.cx-box-main') ||
						document.querySelector('.cx-box-container') ||
						document.querySelector('.cx-box');
					if (container) {
						var icons = container.querySelectorAll('img, i, svg');
						var cRect = container.getBoundingClientRect();
						for (var j = 0; j < icons.length; j++) {
							var r = icons[j].getBoundingClientRect();
							if (r.top < cRect.top + 80 && r.right > cRect.right - 80 &&
								r.width > 0 && r.width < 60) {
								icons[j].click();
								return "container_icon";
							}
						}
					}

					// Strategy 5: click "其他方式登录" text
					var allEls = document.querySelectorAll('span, div, a, p');
					for (var k = 0; k < allEls.length; k++) {
						if (allEls[k].textContent.trim() === '其他方式登录') {
							allEls[k].click();
							return "other_login_text";
						}
					}
					return "not_found";
				})()
			`, &switchResult),
			chromedp.Sleep(2*time.Second),
		); err != nil {
			logger.Warnf("switch to password login failed (non-fatal): %v", err)
		} else {
			logger.Infof("switch to password login result: %s", switchResult)
		}

		// Poll for mobile input to appear (up to 15 seconds)
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			for _, sel := range mobileSelectors {
				var nodes []*cdp.Node
				if err := chromedp.Run(ctx, chromedp.Nodes(sel, &nodes, chromedp.AtLeast(0))); err == nil && len(nodes) > 0 {
					mobileSel = sel
					logger.Infof("mobile input found with selector: %s", sel)
					break
				}
			}
			if mobileSel != "" {
				break
			}
			time.Sleep(1 * time.Second)
		}
	}

	if mobileSel == "" {
		var pageHTML string
		_ = chromedp.Run(ctx, chromedp.OuterHTML("html", &pageHTML))
		if len(pageHTML) > 2000 {
			pageHTML = pageHTML[:2000]
		}
		logger.Errorf("[caixin] login: no mobile input found. Page HTML (truncated): %s", pageHTML)
		return a.wrapStepErrWithScreenshot(ctx, "login_no_mobile_input", fmt.Errorf("no mobile input selector matched"))
	}

	logger.Infof("next step: wait mobile input to be visible")
	err := runStep("wait_mobile_input_visible",
		chromedp.WaitVisible(mobileSel),
	)
	if err != nil {
		return err
	}
	err = runStep("focus_mobile_input",
		chromedp.Focus(mobileSel),
	)
	if err != nil {
		return err
	}
	logger.Infof("next step: clear mobile input")
	err = runStep("clear_mobile_input",
		chromedp.Evaluate(fmt.Sprintf(`document.querySelector("%s").value = ""`, mobileSel), nil),
	)
	if err != nil {
		return err
	}
	logger.Infof("next step: sending mobile number")
	err = runStep("input_mobile_number",
		chromedp.SendKeys(mobileSel, username),
		chromedp.Sleep(1*time.Second),
	)
	if err != nil {
		return err
	}

	// Find password input
	passwordSel := `input[name='password']`
	var pwNodes []*cdp.Node
	if err := chromedp.Run(ctx, chromedp.Nodes(passwordSel, &pwNodes, chromedp.AtLeast(0))); err != nil || len(pwNodes) == 0 {
		passwordSel = `input[type='password']`
	}
	logger.Infof("next step: send password (selector: %s)", passwordSel)
	err = runStep("input_password",
		chromedp.SendKeys(passwordSel, password),
		chromedp.Sleep(1*time.Second),
	)
	if err != nil {
		return err
	}

	// Click consent/agreement checkbox using JavaScript for resilience
	logger.Infof("next step: click agreement checkbox")
	err = runStep("click_agreement_checkbox",
		chromedp.Evaluate(`
			(function(){
				var cb = document.querySelector(".cx-agree-check input[type='checkbox']");
				if (cb) { cb.click(); return "checkbox"; }
				var span = document.querySelector(".cx-agree-check label span span");
				if (span) { span.click(); return "span"; }
				var label = document.querySelector(".cx-agree-check label");
				if (label) { label.click(); return "label"; }
				var cb2 = document.querySelector(".cx-login-argree input[type='checkbox']");
				if (cb2) { cb2.click(); return "checkbox_legacy"; }
				var label2 = document.querySelector(".cx-login-argree label");
				if (label2) { label2.click(); return "label_legacy"; }
				var el = document.querySelector("#app > div > section > div > div.cx-login-argree > label > span > span");
				if (el) { el.click(); return "legacy"; }
				return "not_found";
			})()
		`, nil),
		chromedp.Sleep(1*time.Second),
	)
	if err != nil {
		logger.Warnf("click_agreement_checkbox failed (non-fatal): %v", err)
	}

	// Click login button
	loginBtnSel := `button.login-btn`
	var btnNodes []*cdp.Node
	if err := chromedp.Run(ctx, chromedp.Nodes(loginBtnSel, &btnNodes, chromedp.AtLeast(0))); err != nil || len(btnNodes) == 0 {
		loginBtnSel = `button[type='submit']`
	}
	logger.Infof("next step: click login button (selector: %s)", loginBtnSel)
	err = runStep("click_login_button",
		chromedp.Click(loginBtnSel),
	)
	if err != nil {
		return err
	}
	err = runStep("wait_login_button_disappear",
		chromedp.WaitNotPresent(loginBtnSel),
	)
	if err != nil {
		return err
	}

	return nil
}

func (a *Caixin) wrapStepErrWithScreenshot(ctx context.Context, step string, err error) error {
	screenshotPath, screenshotErr := a.captureDiagnosticScreenshot(ctx, "caixin_"+step)
	if screenshotErr != nil {
		return fmt.Errorf("%s failed: %w (also failed to capture screenshot: %v)", step, err, screenshotErr)
	}
	return fmt.Errorf("%s failed: %w (screenshot: %s)", step, err, screenshotPath)
}

func (a *Caixin) captureDiagnosticScreenshot(ctx context.Context, prefix string) (string, error) {
	var image []byte
	if err := chromedp.Run(ctx, chromedp.FullScreenshot(&image, 90)); err != nil {
		return "", err
	}

	if err := os.MkdirAll("data/diagnostics", 0755); err != nil {
		return "", err
	}

	fileName := fmt.Sprintf("%s_%s.png", prefix, time.Now().Format("20060102_150405"))
	filePath := filepath.Join("data/diagnostics", fileName)
	if err := os.WriteFile(filePath, image, 0644); err != nil {
		return "", err
	}
	return filePath, nil
}

func (a *Caixin) EventListener(ctx context.Context) func(ev interface{}) {
	return func(ev interface{}) {
		if ev, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			logger.Warnf("[%s] Reveived dialog message: %+v", a.Name(), ev)
			// 当检测到弹窗时，自动点击确定按钮（当前用于财新被动登出时的弹窗）
			go func() {
				err := chromedp.Run(ctx,
					page.HandleJavaScriptDialog(true),
				)
				if err != nil {
					logger.Errorf("[caixin] HandleJavaScriptDialog failed: %v", err)
				}
			}()
		}
	}
}
