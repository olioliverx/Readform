package main

import (
	"fmt"
	"testing"
)

func TestPersistence(t *testing.T) {
	restore := setupTestDB(t)
	defer restore()

	if err := addArticle("http://example.com", "agent1", ""); err != nil {
		t.Fatalf("addArticle failed: %v", err)
	}
	if err := markURLAsSaved("http://example2.com", "agent2", "response2"); err != nil {
		t.Fatalf("markURLAsSaved failed: %v", err)
	}

	article, err := findArticle([]string{"http://example.com"}, false, false, false)
	if err != nil {
		t.Fatalf("findArticle failed: %v", err)
	}
	fmt.Println(article)
}
