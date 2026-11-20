package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/service"
)

// TemplateRepository implements service.TemplateRepository.
type TemplateRepository struct {
	pool *pgxpool.Pool
}

// NewTemplateRepository returns a repository using pool.
func NewTemplateRepository(pool *pgxpool.Pool) *TemplateRepository {
	return &TemplateRepository{pool: pool}
}

var _ service.TemplateRepository = (*TemplateRepository)(nil)

const (
	templateColumns   = `id, name, channel, body, created_at, updated_at`
	templateNameIndex = "templates_name_key"
)

func scanTemplate(row pgx.Row) (domain.Template, error) {
	var (
		t       domain.Template
		channel string
	)
	if err := row.Scan(&t.ID, &t.Name, &channel, &t.Body, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return domain.Template{}, err
	}
	t.Channel = domain.Channel(channel)
	t.CreatedAt, t.UpdatedAt = t.CreatedAt.UTC(), t.UpdatedAt.UTC()
	return t, nil
}

func scanTemplateRow(row pgx.CollectableRow) (domain.Template, error) { return scanTemplate(row) }

// Create stores a new template.
func (r *TemplateRepository) Create(ctx context.Context, t domain.Template) (domain.Template, error) {
	stored, err := scanTemplate(r.pool.QueryRow(ctx, `
		INSERT INTO templates (`+templateColumns+`) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+templateColumns,
		t.ID, t.Name, t.Channel, t.Body, t.CreatedAt, t.UpdatedAt))
	if uniqueViolation(err, templateNameIndex) {
		return domain.Template{}, fmt.Errorf("template %q: %w", t.Name, domain.ErrTemplateNameTaken)
	}
	if err != nil {
		return domain.Template{}, fmt.Errorf("insert template: %w", err)
	}
	return stored, nil
}

// Get returns the template with the given ID.
func (r *TemplateRepository) Get(ctx context.Context, id uuid.UUID) (domain.Template, error) {
	t, err := scanTemplate(r.pool.QueryRow(ctx, `SELECT `+templateColumns+` FROM templates WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Template{}, domain.NotFound("template", id.String())
	}
	if err != nil {
		return domain.Template{}, fmt.Errorf("get template: %w", err)
	}
	return t, nil
}

// GetMany returns the templates that exist among ids.
func (r *TemplateRepository) GetMany(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Template, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+templateColumns+` FROM templates WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("get templates: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanTemplateRow)
	if err != nil {
		return nil, fmt.Errorf("get templates: %w", err)
	}
	out := make(map[uuid.UUID]domain.Template, len(list))
	for _, t := range list {
		out[t.ID] = t
	}
	return out, nil
}

// List returns every template ordered by name.
func (r *TemplateRepository) List(ctx context.Context) ([]domain.Template, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+templateColumns+` FROM templates ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanTemplateRow)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	return list, nil
}

// Update replaces a template's name, channel and body.
func (r *TemplateRepository) Update(ctx context.Context, t domain.Template) (domain.Template, error) {
	stored, err := scanTemplate(r.pool.QueryRow(ctx, `
		UPDATE templates SET name = $2, channel = $3, body = $4, updated_at = $5
		WHERE id = $1
		RETURNING `+templateColumns,
		t.ID, t.Name, t.Channel, t.Body, t.UpdatedAt))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.Template{}, domain.NotFound("template", t.ID.String())
	case uniqueViolation(err, templateNameIndex):
		return domain.Template{}, fmt.Errorf("template %q: %w", t.Name, domain.ErrTemplateNameTaken)
	case err != nil:
		return domain.Template{}, fmt.Errorf("update template: %w", err)
	}
	return stored, nil
}

// Delete removes a template. Notifications keep their rendered content; the
// foreign key clears their link to the template.
func (r *TemplateRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM templates WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("template", id.String())
	}
	return nil
}
