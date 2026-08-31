package utils

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/PuerkitoBio/goquery"
)

// GetHTMLInputValue get value for HTML Input
//
// Ex: <input type="hidden" name="username" value="JohnDoe">
// GetHTMLInputValue(resp, "username")
func GetHTMLInputValue(response *http.Response, inputName string) (result string, err error) {
	if response == nil || response.Body == nil {
		return "", errors.New("nil HTTP response")
	}
	defer response.Body.Close()

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		return "", fmt.Errorf("cannot parse HTML: %w", err)
	}

	// Match input elements by name in Go rather than building a CSS selector
	// from untrusted input.
	var value string
	found := false
	doc.Find("input").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if name, _ := s.Attr("name"); name == inputName {
			value, found = s.Attr("value")
			return false
		}
		return true
	})
	if !found {
		return "", fmt.Errorf("input name %s not found", inputName)
	}
	return value, nil
}
