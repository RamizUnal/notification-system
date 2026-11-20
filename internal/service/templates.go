package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/domain"
)

// TemplateService implements the template use cases.
type TemplateService struct {
	settings
	templates TemplateRepository
}

// NewTemplateService wires the service to its repository.
func NewTemplateService(templates TemplateRepository, opts ...Option) *TemplateService {
	return &TemplateService{settings: newSettings(opts), templates: templates}
}

// Create validates and stores a new template.
func (s *TemplateService) Create(ctx context.Context, d domain.TemplateDraft) (domain.Template, error) {
	if err := d.Validate(); err != nil {
		return domain.Template{}, err
	}
	now := s.now().UTC()
	return s.templates.Create(ctx, domain.Template{
		ID:        s.newID(),
		Name:      d.Name,
		Channel:   d.Channel,
		Body:      d.Body,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// Update replaces a template's name, channel and body. Notifications that
// were already rendered from it keep their content.
func (s *TemplateService) Update(ctx context.Context, id uuid.UUID, d domain.TemplateDraft) (domain.Template, error) {
	if err := d.Validate(); err != nil {
		return domain.Template{}, err
	}
	t, err := s.templates.Get(ctx, id)
	if err != nil {
		return domain.Template{}, err
	}
	t.Name, t.Channel, t.Body = d.Name, d.Channel, d.Body
	t.UpdatedAt = s.now().UTC()
	return s.templates.Update(ctx, t)
}

// Get returns one template.
func (s *TemplateService) Get(ctx context.Context, id uuid.UUID) (domain.Template, error) {
	return s.templates.Get(ctx, id)
}

// List returns every template, ordered by name.
func (s *TemplateService) List(ctx context.Context) ([]domain.Template, error) {
	return s.templates.List(ctx)
}

// Delete removes a template. Notifications rendered from it are unaffected.
func (s *TemplateService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.templates.Delete(ctx, id)
}

// Render previews a template with the given variables without creating a
// notification. Missing variables are reported as a validation error.
func (s *TemplateService) Render(ctx context.Context, id uuid.UUID, vars map[string]string) (string, error) {
	t, err := s.templates.Get(ctx, id)
	if err != nil {
		return "", err
	}
	content, err := t.Render(vars)
	if err != nil {
		var missing *domain.MissingVariablesError
		if errors.As(err, &missing) {
			verr := &domain.ValidationError{}
			verr.Add("variables", domain.CodeMissingVariables, err.Error())
			return "", verr
		}
		return "", fmt.Errorf("render template %s: %w", id, err)
	}
	return content, nil
}
