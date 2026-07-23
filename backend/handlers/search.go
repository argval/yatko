package handlers

import (
	"log"
	"net/http"
	"unicode/utf8"

	"github.com/argval/yatko/search"
	"github.com/gin-gonic/gin"
)

// SearchHandler serves GET /api/search?q= — GitHub repo autocomplete for the homepage.
type SearchHandler struct {
	autocomplete *search.Autocomplete
}

func NewSearchHandler(autocomplete *search.Autocomplete) *SearchHandler {
	return &SearchHandler{autocomplete: autocomplete}
}

func (h *SearchHandler) Handle(c *gin.Context) {
	q := search.NormalizeQuery(c.Query("q"))
	if utf8.RuneCountInString(q) < search.MinQueryLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query must be at least 2 characters"})
		return
	}
	if utf8.RuneCountInString(q) > search.MaxQueryLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query too long"})
		return
	}

	items, err := h.autocomplete.Suggest(c.Request.Context(), q)
	if err != nil {
		log.Printf("search: error for %q: %v", q, err)
		c.JSON(httpStatusFromError(err), gin.H{"error": publicErrorMessage(err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}
