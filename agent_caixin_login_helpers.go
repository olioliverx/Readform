package main

import (
	"context"
	"fmt"
	"github.com/chromedp/chromedp"
	"time"
)

var caixinUsernameInputSelectors = []string{
	`input[name='mobile']`,
	`input[name='phone']`,
	`input[name='username']`,
	`input[name='account']`,
	`input[id='mobile']`,
	`input[type='tel']`,
	`input[autocomplete='username']`,
	`input[placeholder="请输入手机号"]`,
	`input[placeholder*='手机号']`,
	`input[placeholder*='手机']`,
	`input[placeholder*='号码']`,
	`input[placeholder*='账号']`,
	`input[placeholder*='账户']`,
	`input[placeholder*='邮箱']`,
}

var caixinPasswordInputSelectors = []string{
	`input[name='password']`,
	`input[type='password']`,
}

var caixinLoginButtonSelectors = []string{
	`button.login-btn`,
	`button[type='submit']`,
	`button[class*='login']`,
	`.login-btn`,
}

func findFirstVisibleSelector(ctx context.Context, selectors []string) (string, error) {
	for _, sel := range selectors {
		visible, err := isSelectorVisible(ctx, sel)
		if err != nil {
			continue
		}
		if visible {
			return sel, nil
		}
	}
	return "", nil
}

func waitForVisibleSelector(ctx context.Context, selectors []string, timeout time.Duration, onRetry func(attempt int)) (string, error) {
	deadline := time.Now().Add(timeout)
	attempt := 0
	for time.Now().Before(deadline) {
		if onRetry != nil {
			onRetry(attempt)
		}
		sel, err := findFirstVisibleSelector(ctx, selectors)
		if err != nil {
			return "", err
		}
		if sel != "" {
			return sel, nil
		}
		attempt++
		time.Sleep(1 * time.Second)
	}
	return "", nil
}

func isSelectorVisible(ctx context.Context, selector string) (bool, error) {
	expr := fmt.Sprintf(`(function() {
		var el = document.querySelector(%q);
		if (!el) return false;
		var style = window.getComputedStyle(el);
		if (!style) return false;
		if (style.display === 'none' || style.visibility === 'hidden') return false;
		var rect = el.getBoundingClientRect();
		return rect.width > 0 && rect.height > 0;
	})()`, selector)
	var visible bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &visible)); err != nil {
		return false, err
	}
	return visible, nil
}

func switchCaixinToPasswordLogin(ctx context.Context) (string, error) {
	var result string
	err := chromedp.Run(ctx, chromedp.Evaluate(`
		(function(){
			var targets = ['账号密码登录', '密码登录', '手机号登录', '账号登录', '其他方式登录'];
			function isVisible(el) {
				if (!el) return false;
				var style = window.getComputedStyle(el);
				if (!style || style.display === 'none' || style.visibility === 'hidden') return false;
				var r = el.getBoundingClientRect();
				return r.width > 0 && r.height > 0;
			}

			var mobileTab = document.getElementById('tab-mobile');
			if (isVisible(mobileTab)) { mobileTab.click(); return 'tab-mobile'; }

			var switcher = document.querySelector('.cx-icon-switch') ||
				document.querySelector('.cx-login-switch') ||
				document.querySelector('.login-switch') ||
				document.querySelector('.qr-switch');
			if (isVisible(switcher)) { switcher.click(); return 'switcher_icon'; }

			var tabs = document.querySelectorAll('.el-tabs__item, [role="tab"], .cx-tab');
			for (var t = 0; t < targets.length; t++) {
				for (var i = 0; i < tabs.length; i++) {
					var tabText = (tabs[i].textContent || '').trim();
					if (tabText.indexOf(targets[t]) >= 0 && isVisible(tabs[i])) {
						tabs[i].click();
						return 'tab_text:' + tabText;
					}
				}
			}

			var allEls = document.querySelectorAll('button, a, span, div, p, li');
			for (var t2 = 0; t2 < targets.length; t2++) {
				for (var k = 0; k < allEls.length; k++) {
					var txt = (allEls[k].textContent || '').trim();
					if (txt.indexOf(targets[t2]) >= 0 && isVisible(allEls[k])) {
						allEls[k].click();
						return 'text:' + txt;
					}
				}
			}
			return 'not_found';
		})()
	`, &result))
	if err != nil {
		return "", err
	}
	return result, nil
}
