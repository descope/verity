package repositoryops

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrInvalidGitHubRepository = errors.New("invalid GitHub repository")
	githubOwnerPattern         = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
	githubRepositoryPattern    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

type githubRepository struct {
	owner string
	name  string
}

func parseGitHubRepository(value string) (githubRepository, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, "/")
	if len(parts) != 2 || !githubOwnerPattern.MatchString(parts[0]) || !githubRepositoryPattern.MatchString(parts[1]) || strings.HasSuffix(parts[1], ".git") {
		return githubRepository{}, fmt.Errorf("%w: expected owner/repository, got %q", ErrInvalidGitHubRepository, value)
	}
	return githubRepository{owner: parts[0], name: parts[1]}, nil
}

func (r githubRepository) pullPath(number string) string {
	return "/" + r.owner + "/" + r.name + "/pull/" + number
}
