package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"urlshortener/internal/repository"
	"urlshortener/internal/service"
)

// LinkHandler is the thin HTTP layer: it decodes requests, calls the
// service layer, and encodes responses. It contains no business logic —
// that all lives in service.LinkService.
type LinkHandler struct {
	svc *service.LinkService
}

func NewLinkHandler(svc *service.LinkService) *LinkHandler {
	return &LinkHandler{svc: svc}
}

// createLinkRequest is the expected JSON body for POST /links, e.g.
// {"url": "https://example.com"}. It's a request-shaped type, separate
// from model.Link, because the request and the stored/returned resource
// don't have the same fields (the request has no code/id/created_at yet).
type createLinkRequest struct {
	URL string `json:"url"`
}

// CreateLink handles POST /links: decode the body, delegate to the
// service, translate the result into an HTTP response. Handlers stay this
// thin on purpose — all the actual decision-making (validation, code
// generation) lives in service.LinkService so it can be tested without
// spinning up an HTTP server.
func (h *LinkHandler) CreateLink(w http.ResponseWriter, r *http.Request) {
	var req createLinkRequest
	// json.NewDecoder(r.Body).Decode streams the request body straight into
	// req rather than buffering it all into a []byte first — the idiomatic
	// way to parse a JSON request body in Go.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	link, err := h.svc.CreateLink(r.Context(), req.URL)
	if err != nil {
		if errors.Is(err, service.ErrInvalidURL) {
			writeError(w, http.StatusBadRequest, "url must be an absolute URL, e.g. https://example.com")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create link")
		return
	}

	writeJSON(w, http.StatusCreated, link)
}

// Redirect handles GET /{code}: looks up the long URL, redirects to it,
// and records a click event.
func (h *LinkHandler) Redirect(w http.ResponseWriter, r *http.Request) {
	// r.PathValue reads a named wildcard segment from the route pattern —
	// here, the "{code}" in "GET /{code}" registered in main.go. It's a
	// stdlib feature added in Go 1.22; no router library needed.
	code := r.PathValue("code")

	link, err := h.svc.Resolve(r.Context(), code)
	if err != nil {
		if errors.Is(err, repository.ErrLinkNotFound) {
			writeError(w, http.StatusNotFound, "short link not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to resolve link")
		return
	}

	// Recorded synchronously, before the redirect, to keep this learning
	// project's control flow simple and easy to follow. A busier service
	// would fire this in a goroutine (or a queue) so a slow DB write never
	// delays the redirect the user is waiting on.
	if err := h.svc.RecordClick(r.Context(), link.ID, r.Referer(), r.UserAgent()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record click")
		return
	}

	// http.StatusFound (302) tells the browser "the resource is temporarily
	// at this other URL" — the standard status for a redirect-style short
	// link. http.Redirect sets the Location header and status code itself.
	http.Redirect(w, r, link.LongURL, http.StatusFound)
}

// Stats handles GET /links/{code}/stats.
func (h *LinkHandler) Stats(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	stats, err := h.svc.GetStats(r.Context(), code)
	if err != nil {
		if errors.Is(err, repository.ErrLinkNotFound) {
			writeError(w, http.StatusNotFound, "short link not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// List handles GET /links?limit=&offset=.
func (h *LinkHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)

	links, err := h.svc.ListLinks(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list links")
		return
	}

	writeJSON(w, http.StatusOK, links)
}

// queryInt reads an integer query param, falling back to def if it's
// missing or not a valid integer.
func queryInt(r *http.Request, key string, def int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

// writeJSON encodes v as the JSON response body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a consistent {"error": "..."} JSON body.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
