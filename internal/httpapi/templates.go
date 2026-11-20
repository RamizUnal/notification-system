package httpapi

import (
	"net/http"
	"unicode/utf8"
)

// createTemplate handles POST /api/v1/templates.
func (a *API) createTemplate(w http.ResponseWriter, r *http.Request) error {
	var req templateRequest
	if err := decodeJSON(w, r, &req, maxSmallBody); err != nil {
		return err
	}
	t, err := a.cfg.Templates.Create(r.Context(), req.toDraft())
	if err != nil {
		return err
	}
	w.Header().Set("Location", "/api/v1/templates/"+t.ID.String())
	return writeJSON(w, http.StatusCreated, toTemplateResponse(t))
}

// listTemplates handles GET /api/v1/templates.
func (a *API) listTemplates(w http.ResponseWriter, r *http.Request) error {
	list, err := a.cfg.Templates.List(r.Context())
	if err != nil {
		return err
	}
	data := make([]templateResponse, len(list))
	for i, t := range list {
		data[i] = toTemplateResponse(t)
	}
	return writeJSON(w, http.StatusOK, struct {
		Data []templateResponse `json:"data"`
	}{data})
}

// getTemplate handles GET /api/v1/templates/{id}.
func (a *API) getTemplate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "template")
	if err != nil {
		return err
	}
	t, err := a.cfg.Templates.Get(r.Context(), id)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, toTemplateResponse(t))
}

// updateTemplate handles PUT /api/v1/templates/{id}.
func (a *API) updateTemplate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "template")
	if err != nil {
		return err
	}
	var req templateRequest
	if err := decodeJSON(w, r, &req, maxSmallBody); err != nil {
		return err
	}
	t, err := a.cfg.Templates.Update(r.Context(), id, req.toDraft())
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, toTemplateResponse(t))
}

// deleteTemplate handles DELETE /api/v1/templates/{id}.
func (a *API) deleteTemplate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "template")
	if err != nil {
		return err
	}
	if err := a.cfg.Templates.Delete(r.Context(), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// previewTemplate handles POST /api/v1/templates/{id}/preview: it renders
// the template with sample variables without creating anything.
func (a *API) previewTemplate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "template")
	if err != nil {
		return err
	}
	var req previewRequest
	if err := decodeJSON(w, r, &req, maxSmallBody); err != nil {
		return err
	}
	content, err := a.cfg.Templates.Render(r.Context(), id, req.Variables)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, struct {
		Content string `json:"content"`
		Length  int    `json:"length"`
	}{content, utf8.RuneCountInString(content)})
}
