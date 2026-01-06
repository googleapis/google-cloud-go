package firestore

import (
	"fmt"
	"net/url"
	"regexp"
)

var (
	projectRegex  = regexp.MustCompile("projects/(?P<project_id>[^/]+)(?:/.*)?")
	databaseRegex = regexp.MustCompile("projects/[^/]+/databases/(?P<database_id>[^/]+)(?:/.*)?")
)

func buildRoutingHeader(input string) string {
	var routingHeaders string
	var projectID, databaseID string

	if match := projectRegex.FindStringSubmatch(input); len(match) > 1 {
		projectID = match[1]
	}
	if match := databaseRegex.FindStringSubmatch(input); len(match) > 1 {
		databaseID = match[1]
	}

	if projectID != "" {
		routingHeaders = fmt.Sprintf("project_id=%s", url.QueryEscape(projectID))
	}
	if databaseID != "" {
		if routingHeaders != "" {
			routingHeaders += "&"
		}
		routingHeaders += fmt.Sprintf("database_id=%s", url.QueryEscape(databaseID))
	}
	return routingHeaders
}
