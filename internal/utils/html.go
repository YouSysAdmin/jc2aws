package utils

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// GetHTMLInputValue get value for HTML Input
//
// Ex: <input type="hidden" name="username" value="JohnDoe">
// GetHTMLInputValue(resp, "username")
func GetHTMLInputValue(response *http.Response, inputName string) (result string, err error) {
	defer response.Body.Close()

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		return "", err
	}

	// Escape quotes and backslashes to prevent CSS selector injection
	sanitized := strings.ReplaceAll(inputName, `\`, `\\`)
	sanitized = strings.ReplaceAll(sanitized, `"`, `\"`)
	query := fmt.Sprintf("[name=\"%s\"]", sanitized)
	result, exist := doc.Find(query).Attr("value")
	if !exist {
		return "", fmt.Errorf("input name %s not found", inputName)
	}
	return result, nil
}
