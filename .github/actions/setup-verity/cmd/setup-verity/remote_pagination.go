package main

import (
	"net/url"
	"strings"
)

type artifactPageRequest struct {
	Current      *url.URL
	ExpectedPath string
	ArtifactName string
}

func nextArtifactPage(link string, request artifactPageRequest) (url.URL, error) {
	target, err := nextLinkTarget(link)
	if err != nil || target == "" {
		return url.URL{}, err
	}
	next, err := url.Parse(target)
	if err != nil || next.Scheme != request.Current.Scheme || next.Host != request.Current.Host ||
		next.Path != request.ExpectedPath || next.User != nil {
		return url.URL{}, artifactMismatch("artifact API next link")
	}
	query := next.Query()
	if query.Get("name") != request.ArtifactName || query.Get("per_page") != "100" || query.Get("page") == "" {
		return url.URL{}, artifactMismatch("artifact API next query")
	}
	return *next, nil
}

func nextLinkTarget(link string) (string, error) {
	for value := range strings.SplitSeq(link, ",") {
		segments := strings.Split(value, ";")
		if len(segments) < 2 || strings.TrimSpace(segments[1]) != `rel="next"` {
			continue
		}
		target := strings.TrimSpace(segments[0])
		if len(target) < 3 || target[0] != '<' || target[len(target)-1] != '>' {
			return "", artifactMismatch("artifact API next link")
		}
		return target[1 : len(target)-1], nil
	}
	return "", nil
}
