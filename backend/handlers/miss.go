package handlers

import (
	"log"
	"net/http"
	"strings"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

// IsScriptUA reports whether ua looks like curl/wget/CI HTTP clients rather
// than a browser. Empty UA is treated as a script (common for some tools).
func IsScriptUA(ua string) bool {
	if strings.TrimSpace(ua) == "" {
		return true
	}
	lower := strings.ToLower(ua)
	markers := []string{
		"curl/",
		"wget",
		"httpie",
		"go-http-client",
		"python-requests",
		"python-urllib",
		"axios/",
		"node-fetch",
		"http.rb",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

const maxMissAssetNames = 20

func logPickerMiss(
	owner, repo string,
	platform picker.Platform,
	arch picker.Arch,
	prefer string,
	libc picker.Libc,
	script bool,
	assets []github.Asset,
	decision picker.AssetDecision,
) {
	n := len(assets)
	names := make([]string, 0, maxMissAssetNames)
	for i, a := range assets {
		if i >= maxMissAssetNames {
			break
		}
		names = append(names, a.Name)
	}
	guess := ""
	if decision.Asset != nil {
		guess = decision.Asset.Name
	}
	log.Printf(
		"picker_miss owner=%s repo=%s platform=%s arch=%s prefer=%s libc=%s script=%v confidence=%s guess=%q reasons=%q assets=%d names=%q",
		owner, repo, platform, arch, prefer, libc, script, decision.Confidence, guess, decision.Reasons, n, names,
	)
}

// respondDownloadMiss handles /dl when no asset matched or confidence is too
// low to auto-select: scripts get 404 JSON; browsers keep the historical
// redirect to the GitHub release HTML page.
func respondDownloadMiss(
	c *gin.Context,
	owner, repo string,
	release *github.Release,
	platform picker.Platform,
	arch picker.Arch,
	prefer string,
	libc picker.Libc,
	decision picker.AssetDecision,
) {
	script := IsScriptUA(c.GetHeader("User-Agent"))
	logPickerMiss(owner, repo, platform, arch, prefer, libc, script, release.Assets, decision)
	if script {
		c.JSON(http.StatusNotFound, gin.H{
			"error":    "no suitable asset found for platform",
			"platform": string(platform),
			"arch":     string(arch),
			"url":      release.HTMLURL,
		})
		return
	}
	c.Redirect(http.StatusFound, release.HTMLURL)
}
