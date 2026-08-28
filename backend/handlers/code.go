package handlers

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

// HandleCode redirects to GitHub's ZIP source archive. An optional tag pins
// the archive to a release; without one, GitHub resolves the default branch.
func HandleCode(c *gin.Context) {
	c.Redirect(http.StatusFound, codeArchiveURL(c.Param("owner"), c.Param("repo"), c.Query("tag")))
}

func codeArchiveURL(owner, repo, tag string) string {
	base := fmt.Sprintf("https://github.com/%s/%s/archive/", url.PathEscape(owner), url.PathEscape(repo))
	if tag == "" {
		return base + "HEAD.zip"
	}
	return base + "refs/tags/" + url.PathEscape(tag) + ".zip"
}
